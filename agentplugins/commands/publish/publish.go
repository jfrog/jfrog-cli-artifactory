package publish

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/agentcommon"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agentplugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// evidenceLicenseErrFragment is the substring in error messages that indicates
// the Artifactory instance lacks the Enterprise+ license required for evidence.
const evidenceLicenseErrFragment = "Enterprise+"

var zipExcludes = map[string]bool{
	".git":         true,
	".jfrog":       true,
	"__pycache__":  true,
	"node_modules": true,
	".DS_Store":    true,
}

type PublishCommand struct {
	serverDetails      *config.ServerDetails
	repoKey            string
	pluginDir          string
	version            string
	signingKey         string
	keyAlias           string
	quiet              bool
	buildConfiguration *build.BuildConfiguration
}

func NewPublishCommand() *PublishCommand { return &PublishCommand{} }

func (pc *PublishCommand) SetServerDetails(d *config.ServerDetails) *PublishCommand {
	pc.serverDetails = d
	return pc
}
func (pc *PublishCommand) SetRepoKey(k string) *PublishCommand   { pc.repoKey = k; return pc }
func (pc *PublishCommand) SetPluginDir(d string) *PublishCommand { pc.pluginDir = d; return pc }
func (pc *PublishCommand) SetVersion(v string) *PublishCommand   { pc.version = v; return pc }
func (pc *PublishCommand) SetSigningKey(p string) *PublishCommand {
	pc.signingKey = p
	return pc
}
func (pc *PublishCommand) SetKeyAlias(a string) *PublishCommand { pc.keyAlias = a; return pc }
func (pc *PublishCommand) SetQuiet(q bool) *PublishCommand      { pc.quiet = q; return pc }
func (pc *PublishCommand) SetBuildConfiguration(b *build.BuildConfiguration) *PublishCommand {
	pc.buildConfiguration = b
	return pc
}

func (pc *PublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PublishCommand) CommandName() string { return "ai_plugins_publish" }

func (pc *PublishCommand) Run() error {
	meta, err := plugincommon.ValidateAndResolvePluginMeta(pc.pluginDir, pc.version)
	if err != nil {
		return err
	}

	slug := meta.Name
	if err := plugincommon.ValidateSlug(slug); err != nil {
		return err
	}

	version, err := pc.resolveVersionCollision(slug, meta.Version)
	if err != nil {
		return err
	}
	if err := plugincommon.ValidateVersion(version); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Publishing plugin '%s' version '%s'", slug, version))

	zipPath, sha256Hex, prebuilt, err := pc.resolveZip(slug, version)
	if err != nil {
		return err
	}
	defer func() {
		if !prebuilt {
			_ = os.Remove(zipPath)
		}
	}()
	if sha256Hex == "" {
		// Prebuilt zips bypass the streaming hasher; hash on disk in that case.
		if sha256Hex, err = computeSHA256(zipPath); err != nil {
			return fmt.Errorf("failed to compute SHA256: %w", err)
		}
	}

	collectBuildInfo := false
	if pc.buildConfiguration != nil {
		collectBuildInfo, err = pc.buildConfiguration.IsCollectBuildInfo()
		if err != nil {
			return err
		}
		if collectBuildInfo && pc.buildConfiguration.GetModule() == "" {
			pc.buildConfiguration.SetModule(slug)
		}
	}

	target := fmt.Sprintf("%s/%s/%s/", pc.repoKey, slug, version)
	artifactsDetailsReader, err := pc.upload(zipPath, target, collectBuildInfo)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	if artifactsDetailsReader != nil {
		defer func() { _ = artifactsDetailsReader.Close() }()
		buildArtifacts, err := rtServicesUtils.ConvertArtifactsDetailsToBuildInfoArtifacts(artifactsDetailsReader)
		if err != nil {
			return fmt.Errorf("failed to convert artifacts for build-info: %w", err)
		}
		if err := build.PopulateBuildArtifactsAsPartials(buildArtifacts, pc.buildConfiguration, entities.Generic); err != nil {
			return fmt.Errorf("failed to save build-info partials: %w", err)
		}
	}

	log.Info("Upload complete. Attaching evidence...")
	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s", pc.repoKey, slug, version, filepath.Base(zipPath))
	pc.attachEvidence(slug, version, sha256Hex, subjectRepoPath)

	log.Info(fmt.Sprintf("Plugin '%s' version '%s' published successfully.", slug, version))
	return nil
}

// resolveVersionCollision checks whether the given version already exists in Artifactory.
// In interactive mode the user picks: overwrite, enter a new version, or abort.
// In quiet/CI mode it fails so pipelines don't silently overwrite artifacts.
func (pc *PublishCommand) resolveVersionCollision(slug, version string) (string, error) {
	exists, err := agentcommon.PackageVersionExists(pc.serverDetails, pc.repoKey, slug, version)
	if err != nil {
		// In CI/quiet mode, refuse to proceed on an inconclusive check so we don't
		// silently overwrite. Interactive callers get a debug log and continue.
		if pc.quiet {
			return "", fmt.Errorf("could not verify whether version %s of plugin '%s' already exists: %w", version, slug, err)
		}
		log.Debug("Could not check version existence:", err.Error())
		return version, nil
	}
	if !exists {
		return version, nil
	}

	if pc.quiet {
		return "", fmt.Errorf("version %s of plugin '%s' already exists. Use a different version or remove the existing one", version, slug)
	}

	log.Warn(fmt.Sprintf("Version %s of plugin '%s' already exists in repository '%s'.", version, slug, pc.repoKey))
	fmt.Println("Choose an action:")
	fmt.Println("  [o] Overwrite the existing version")
	fmt.Println("  [n] Enter a new version")
	fmt.Println("  [a] Abort")
	fmt.Print("Your choice (o/n/a): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(strings.ToLower(input))

	switch choice {
	case "o":
		log.Info(fmt.Sprintf("Overwriting version %s...", version))
		return version, nil
	case "n":
		fmt.Print("Enter new version: ")
		newInput, _ := reader.ReadString('\n')
		newVersion := strings.TrimSpace(newInput)
		if newVersion == "" {
			return "", fmt.Errorf("no version provided, aborting")
		}
		if err := plugincommon.ValidateVersion(newVersion); err != nil {
			return "", err
		}
		return pc.resolveVersionCollision(slug, newVersion)
	default:
		return "", fmt.Errorf("publish aborted by user")
	}
}

// resolveZip locates or builds the publish zip and, when it was built locally,
// also returns its SHA256 (computed in the same pass as the write). The prebuilt
// flag indicates whether the path is a user-managed file (must not be deleted).
func (pc *PublishCommand) resolveZip(slug, version string) (zipPath, sha256Hex string, prebuilt bool, err error) {
	if strings.Contains(version, "..") || strings.ContainsAny(version, "/\\") {
		return "", "", false, fmt.Errorf("invalid version '%s': contains path traversal characters", version)
	}
	candidate := filepath.Clean(filepath.Join(pc.pluginDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version)))
	if _, statErr := os.Stat(candidate); statErr == nil {
		log.Info("Using pre-built zip:", candidate)
		return candidate, "", true, nil
	}
	zipPath, sha256Hex, err = zipPluginFolder(pc.pluginDir, slug, version)
	return zipPath, sha256Hex, false, err
}

var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type pluginFile struct {
	relPath string
	mode    os.FileMode
}

func collectFiles(pluginDir string) (files []pluginFile, maxMtime time.Time, err error) {
	err = filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		if shouldExclude(info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			files = append(files, pluginFile{relPath: relPath, mode: info.Mode()})
			if info.ModTime().After(maxMtime) {
				maxMtime = info.ModTime()
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })
	return
}

func addFileToZip(w *zip.Writer, pluginDir string, pf pluginFile, uniformTime time.Time) error {
	absPath := filepath.Join(pluginDir, pf.relPath)

	header := &zip.FileHeader{
		Name:     pf.relPath,
		Method:   zip.Deflate,
		Modified: uniformTime,
	}
	header.SetModTime(uniformTime) //nolint:staticcheck // sets legacy MS-DOS ModifiedDate/ModifiedTime fields
	header.SetMode(normalizeFileMode(pf.mode))
	header.Extra = nil

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}
	// #nosec G304 -- absPath is from user-provided plugin directory joined with a walked relative path.
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(writer, file)
	return err
}

func normalizeFileMode(mode os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o644
	}
	return mode
}

func zipPluginFolder(pluginDir, slug, version string) (zipPath, sha256Hex string, err error) {
	files, maxMtime, err := collectFiles(pluginDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to collect plugin files: %w", err)
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("no files found in plugin directory %s (all files may have been excluded)", pluginDir)
	}
	if maxMtime.IsZero() {
		maxMtime = zipEpoch
	}

	tmpDir, err := os.MkdirTemp("", "agent-plugin-publish-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	zipPath = filepath.Clean(filepath.Join(tmpDir, fmt.Sprintf("%s-%s.zip", slug, version)))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	hasher := sha256.New()
	w := zip.NewWriter(io.MultiWriter(zipFile, hasher))
	defer func() {
		if cerr := w.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to finalize zip: %w", cerr)
		}
		if err == nil {
			sha256Hex = hex.EncodeToString(hasher.Sum(nil))
		}
	}()

	for _, pf := range files {
		if err = addFileToZip(w, pluginDir, pf, maxMtime); err != nil {
			return "", "", fmt.Errorf("failed to add %s to zip: %w", pf.relPath, err)
		}
	}
	return
}

func shouldExclude(info os.FileInfo) bool {
	name := info.Name()
	return zipExcludes[name] || strings.HasSuffix(name, ".pyc")
}

func computeSHA256(path string) (string, error) {
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("invalid path: contains traversal sequence")
	}
	cleanPath := filepath.Clean(path)
	// #nosec G304 -- cleanPath is derived from a path produced inside this package.
	f, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (pc *PublishCommand) upload(zipPath, target string, collectBuildInfo bool) (*content.ContentReader, error) {
	serviceManager, err := utils.CreateUploadServiceManager(pc.serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return nil, err
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = zipPath
	uploadParams.Target = target
	uploadParams.Flat = true

	if collectBuildInfo {
		if pc.buildConfiguration == nil {
			return nil, fmt.Errorf("build-info collection requested, but build configuration is nil")
		}
		buildProps, err := build.CreateBuildPropsFromConfiguration(pc.buildConfiguration)
		if err != nil {
			return nil, err
		}
		uploadParams.BuildProps = buildProps

		summary, err := serviceManager.UploadFilesWithSummary(artifactory.UploadServiceOptions{}, uploadParams)
		if err != nil {
			return nil, err
		}
		if summary != nil {
			if summary.TransferDetailsReader != nil {
				_ = summary.TransferDetailsReader.Close()
			}
			return summary.ArtifactsDetailsReader, nil
		}
		return nil, nil
	}

	_, _, err = serviceManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	return nil, err
}

func (pc *PublishCommand) attachEvidence(slug, version, sha256Hex, subjectRepoPath string) {
	keyPath := pc.signingKey
	if keyPath == "" {
		keyPath = os.Getenv("EVD_SIGNING_KEY_PATH")
	}
	if keyPath == "" {
		keyPath = os.Getenv("JFROG_CLI_SIGNING_KEY")
	}

	alias := pc.keyAlias
	if alias == "" {
		alias = os.Getenv("EVD_KEY_ALIAS")
	}

	if keyPath == "" {
		log.Info("No signing key configured. Provide --signing-key flag or set EVD_SIGNING_KEY_PATH env var. Skipping evidence creation.")
		return
	}

	tmpDir, err := os.MkdirTemp("", "agent-plugin-evidence-*")
	if err != nil {
		log.Warn("Failed to create temp dir for evidence:", err.Error())
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	publishedAt := time.Now()
	predicatePath, err := GeneratePredicateFile(tmpDir, slug, version, publishedAt)
	if err != nil {
		log.Warn("Failed to generate predicate:", err.Error())
		return
	}

	markdownPath, err := GenerateMarkdownFile(tmpDir, slug, version, publishedAt)
	if err != nil {
		log.Warn("Failed to generate attestation markdown:", err.Error())
		return
	}

	opts := agentcommon.CreateEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
		SubjectSHA256:   sha256Hex,
		PredicatePath:   predicatePath,
		PredicateType:   predicateTypePublishAttestation,
		MarkdownPath:    markdownPath,
		KeyPath:         keyPath,
		KeyAlias:        alias,
	}

	err = withSuppressedLogs(func() error {
		return agentcommon.CreateEvidence(pc.serverDetails, opts)
	})
	if err != nil {
		if isEvidenceLicenseError(err) {
			log.Info("Evidence not attached: evidence requires an Enterprise+ license. Plugin upload succeeded.")
		} else {
			log.Warn("Evidence creation failed (plugin upload succeeded):", err.Error())
		}
		return
	}

	log.Info("Evidence successfully attached.")
}

func withSuppressedLogs(fn func() error) error {
	if jfLogger, ok := log.GetLogger().(*log.JfrogLogger); ok {
		prev := jfLogger.GetLogLevel()
		jfLogger.SetLogLevel(-1)
		defer jfLogger.SetLogLevel(prev)
	}
	return fn()
}

func isEvidenceLicenseError(err error) bool {
	return strings.Contains(err.Error(), evidenceLicenseErrFragment)
}

// RunPublish is the standalone CLI action for `jf ai-plugins publish <path>`.
// It expects the first positional arg to be the plugin directory.
func RunPublish(c *components.Context) error {
	return runPublishAt(c, 0, "jf ai-plugins publish <path-to-plugin-folder> [--repo <repo>] [options]")
}

// RunPublishFromDispatcher is the CLI action when `jf ai plugins publish <path>` is
// invoked via the ai-namespace dispatcher. The first arg is the subcommand name
// ("publish") and the second arg is the plugin directory.
func RunPublishFromDispatcher(c *components.Context) error {
	return runPublishAt(c, 1, "jf ai plugins publish <path-to-plugin-folder> [--repo <repo>] [options]")
}

func runPublishAt(c *components.Context, pathArgIndex int, usage string) error {
	if c.GetNumberOfArgs() <= pathArgIndex {
		return fmt.Errorf("usage: %s", usage)
	}
	pluginDir := c.GetArgumentAt(pathArgIndex)
	absDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return fmt.Errorf("invalid plugin path: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("plugin path '%s' is not a valid directory", pluginDir)
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, agentcommon.AgentPluginsRepoOptions())
	if err != nil {
		return err
	}

	buildConfig, err := pluginsCommon.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return err
	}

	cmd := NewPublishCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetPluginDir(absDir).
		SetVersion(c.GetStringFlagValue("version")).
		SetSigningKey(c.GetStringFlagValue("signing-key")).
		SetKeyAlias(c.GetStringFlagValue("key-alias")).
		SetQuiet(quiet).
		SetBuildConfiguration(buildConfig)

	return cmd.Run()
}
