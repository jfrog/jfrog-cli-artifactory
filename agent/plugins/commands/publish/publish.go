package publish

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// packageVersionExists checks whether a version folder exists in Artifactory. Tests may replace it.
var packageVersionExists = common.PackageVersionExists

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

func NewPublishCommand() *PublishCommand {
	return &PublishCommand{}
}

func (pc *PublishCommand) SetServerDetails(details *config.ServerDetails) *PublishCommand {
	pc.serverDetails = details
	return pc
}

func (pc *PublishCommand) SetRepoKey(repoKey string) *PublishCommand {
	pc.repoKey = repoKey
	return pc
}

func (pc *PublishCommand) SetPluginDir(pluginDir string) *PublishCommand {
	pc.pluginDir = pluginDir
	return pc
}

func (pc *PublishCommand) SetVersion(version string) *PublishCommand {
	pc.version = version
	return pc
}

func (pc *PublishCommand) SetSigningKey(path string) *PublishCommand {
	pc.signingKey = path
	return pc
}

func (pc *PublishCommand) SetKeyAlias(alias string) *PublishCommand {
	pc.keyAlias = alias
	return pc
}

func (pc *PublishCommand) SetQuiet(quiet bool) *PublishCommand {
	pc.quiet = quiet
	return pc
}

func (pc *PublishCommand) SetBuildConfiguration(buildConfig *build.BuildConfiguration) *PublishCommand {
	pc.buildConfiguration = buildConfig
	return pc
}

func (pc *PublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PublishCommand) CommandName() string { return "agent_plugins_publish" }

func (pc *PublishCommand) Run() error {
	meta, err := plugincommon.ValidateAndResolvePluginMeta(pc.pluginDir, pc.version)
	if err != nil {
		return err
	}
	slug := meta.Name
	if err := common.ValidateSlug(slug); err != nil {
		return err
	}
	version, err := pc.resolveVersionCollision(slug, meta.Version)
	if err != nil {
		return err
	}
	if err := common.ValidateSemver(version); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Publishing plugin '%s' version '%s'", slug, version))

	zipPath, sha256Hex, zipTmpDir, _, err := pc.resolveZip(slug, version)
	if err != nil {
		return err
	}
	defer func() {
		if zipTmpDir != "" {
			_ = os.RemoveAll(zipTmpDir) // best-effort temp cleanup after upload
		}
	}()
	if sha256Hex == "" {
		// Prebuilt zips bypass the streaming hasher; hash on disk in that case.
		if sha256Hex, err = common.ComputeSHA256(zipPath); err != nil {
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
	artifactsDetailsReader, err := common.UploadPublishArtifact(pc.serverDetails, zipPath, target, collectBuildInfo, pc.buildConfiguration)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	if artifactsDetailsReader != nil {
		defer func() { _ = artifactsDetailsReader.Close() }() // read-side close after build-info partials
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
// In quiet, CI, or other non-interactive mode it fails so pipelines don't silently overwrite artifacts.
func (pc *PublishCommand) resolveVersionCollision(slug, version string) (string, error) {
	nonInteractive := pc.quiet || common.IsNonInteractive()

	exists, err := packageVersionExists(pc.serverDetails, pc.repoKey, slug, version)
	if err != nil {
		if nonInteractive {
			return "", fmt.Errorf("could not verify whether version %s of plugin '%s' already exists: %w", version, slug, err)
		}
		if errors.Is(err, common.ErrVersionExistenceUnknown) {
			log.Warn("Could not verify whether version exists (Artifactory HTTP status unavailable; 404 detection disabled); proceeding:", err.Error())
		} else {
			log.Debug("Could not check version existence:", err.Error())
		}
		return version, nil
	}
	if !exists {
		return version, nil
	}

	if nonInteractive {
		return "", fmt.Errorf("version %s of plugin '%s' already exists. Use a different version or remove the existing one", version, slug)
	}

	log.Warn(fmt.Sprintf("Version %s of plugin '%s' already exists in repository '%s'.", version, slug, pc.repoKey))
	fmt.Println("Choose an action:")
	fmt.Println("  [o] Overwrite the existing version")
	fmt.Println("  [n] Enter a new version")
	fmt.Println("  [a] Abort")
	fmt.Print("Your choice (o/n/a): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read user input: %w", err)
	}
	choice := strings.TrimSpace(strings.ToLower(input))

	switch choice {
	case "o":
		log.Info(fmt.Sprintf("Overwriting version %s...", version))
		return version, nil
	case "n":
		fmt.Print("Enter new version: ")
		newInput, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read user input: %w", err)
		}
		newVersion := strings.TrimSpace(newInput)
		if newVersion == "" {
			return "", fmt.Errorf("no version provided, aborting")
		}
		if err := common.ValidateSemver(newVersion); err != nil {
			return "", err
		}
		return pc.resolveVersionCollision(slug, newVersion)
	default:
		return "", fmt.Errorf("publish aborted by user")
	}
}

// resolveZip locates or builds the publish zip and, when it was built locally,
// also returns its SHA256 (computed in the same pass as the write) and the temp
// directory holding the zip. zipTmpDir is empty for prebuilt zips; callers should
// defer os.RemoveAll(zipTmpDir) when non-empty.
func (pc *PublishCommand) resolveZip(slug, version string) (zipPath, sha256Hex, zipTmpDir string, prebuilt bool, err error) {
	if common.IsPrebuiltPublishZip(pc.pluginDir, slug, version) {
		prebuiltPath := common.PrebuiltPublishZipPath(pc.pluginDir, slug, version)
		log.Info("Using pre-built zip:", prebuiltPath)
		return prebuiltPath, "", "", true, nil
	}

	zipPath, zipTmpDir, sha256Hex, err = common.ZipPublishBundle(common.ZipPublishOptions{
		SourceDir:      pc.pluginDir,
		Slug:           slug,
		Version:        version,
		TempDirPrefix:  "agent-plugin-publish-",
		ContentLabel:   "plugin",
		HashWhileWrite: true,
	})
	return zipPath, sha256Hex, zipTmpDir, false, err
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
	defer func() { _ = os.RemoveAll(tmpDir) }() // best-effort evidence temp dir cleanup

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
	opts := common.CreateEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
		SubjectSHA256:   sha256Hex,
		PredicatePath:   predicatePath,
		PredicateType:   predicateTypePublishAttestation,
		MarkdownPath:    markdownPath,
		KeyPath:         keyPath,
		KeyAlias:        alias,
	}

	err = common.WithSuppressedLogs(func() error {
		return common.CreateEvidence(pc.serverDetails, opts)
	})
	if err != nil {
		if common.IsEvidenceLicenseError(err) {
			log.Info("Evidence not attached: evidence requires an Enterprise+ license. Plugin upload succeeded.")
		} else {
			log.Warn("Evidence creation failed (plugin upload succeeded):", err.Error())
		}
		return
	}
	log.Info("Evidence successfully attached.")
}

// validatePluginDir resolves pluginDir to an absolute path and ensures it is an existing directory.
func validatePluginDir(pluginDir string) (string, error) {
	absDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return "", fmt.Errorf("invalid plugin path: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("plugin path '%s' is not a valid directory", pluginDir)
	}
	return absDir, nil
}

// RunPublish is the CLI action for `jf agent plugins publish <path>`.
func RunPublish(commandContext *components.Context) error {
	if commandContext.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins publish <path-to-plugin-folder> [--repo <repo>] [options]")
	}
	absDir, err := validatePluginDir(commandContext.GetArgumentAt(0))
	if err != nil {
		return err
	}
	serverDetails, err := common.GetServerDetails(commandContext)
	if err != nil {
		return err
	}
	quiet := common.IsQuiet(commandContext)
	repoKey, err := common.ResolveRepo(serverDetails, commandContext.GetStringFlagValue("repo"), quiet, plugincommon.RepoOptions())
	if err != nil {
		return err
	}
	buildConfig, err := pluginsCommon.CreateBuildConfigurationWithModule(commandContext)
	if err != nil {
		return err
	}
	cmd := NewPublishCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetPluginDir(absDir).
		SetVersion(commandContext.GetStringFlagValue("version")).
		SetSigningKey(commandContext.GetStringFlagValue("signing-key")).
		SetKeyAlias(commandContext.GetStringFlagValue("key-alias")).
		SetQuiet(quiet).
		SetBuildConfiguration(buildConfig)

	return cmd.Run()
}
