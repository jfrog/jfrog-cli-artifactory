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

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

// evidenceLicenseErrFragment is the substring in error messages that indicates
// the Artifactory instance lacks the Enterprise+ license required for evidence.
const evidenceLicenseErrFragment = "Enterprise+"

var zipExcludes = map[string]bool{
	".git":         true,
	"__pycache__":  true,
	"node_modules": true,
	".DS_Store":    true,
}

type PublishCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	skillDir      string
	version       string
	signingKey    string
	keyAlias      string
	quiet         bool
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

func (pc *PublishCommand) SetSkillDir(dir string) *PublishCommand {
	pc.skillDir = dir
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

func (pc *PublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PublishCommand) CommandName() string {
	return "skills_publish"
}

func (pc *PublishCommand) Run() error {
	meta, err := ParseSkillMeta(pc.skillDir)
	if err != nil {
		return err
	}

	slug := meta.Name
	if err := ValidateSlug(slug); err != nil {
		return err
	}

	common.WarnIfXrayDisabled(pc.serverDetails, pc.repoKey)

	version := pc.version
	if version == "" {
		version = meta.Version
	}
	if version == "" {
		version, err = pc.resolveMissingVersion(slug)
		if err != nil {
			return err
		}
	}

	if err := ValidateVersion(version); err != nil {
		return err
	}

	version, err = pc.resolveVersionCollision(slug, version)
	if err != nil {
		return err
	}

	if meta.Version != "" && meta.Version != version {
		if updateErr := UpdateSkillMetaVersion(pc.skillDir, version); updateErr != nil {
			return fmt.Errorf("failed to update SKILL.md version: %w", updateErr)
		}
		log.Info(fmt.Sprintf("Updated SKILL.md version from '%s' to '%s'", meta.Version, version))
	}

	log.Info(fmt.Sprintf("Publishing skill '%s' version '%s'", slug, version))

	zipPath, err := pc.resolveZip(slug, version)
	if err != nil {
		return err
	}
	defer func() {
		if !isPrebuiltZip(pc.skillDir, slug, version) {
			_ = os.Remove(zipPath)
		}
	}()

	sha256Hex, err := computeSHA256(zipPath)
	if err != nil {
		return fmt.Errorf("failed to compute SHA256: %w", err)
	}

	target := fmt.Sprintf("%s/%s/%s/", pc.repoKey, slug, version)
	if err := pc.upload(zipPath, target); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	log.Info("Upload complete. Attaching evidence...")
	pc.attachEvidence(slug, version, sha256Hex)

	log.Info(fmt.Sprintf("Skill '%s' version '%s' published successfully.", slug, version))
	return nil
}

// resolveMissingVersion handles the case where neither --version nor SKILL.md frontmatter
// provides a version. It fetches existing versions from Artifactory, then:
//   - Interactive: shows them and asks the user to enter a version
//   - CI/quiet: auto-increments to the next minor version (or defaults to 0.1.0)
func (pc *PublishCommand) resolveMissingVersion(slug string) (string, error) {
	versions, err := common.ListVersions(pc.serverDetails, pc.repoKey, slug)
	if err != nil {
		log.Debug("Could not fetch existing versions:", err.Error())
	}

	versionStrs := make([]string, len(versions))
	for i, v := range versions {
		versionStrs[i] = v.Version
	}

	if len(versionStrs) > 0 {
		latest, _ := common.LatestVersion(versionStrs)
		if pc.quiet {
			next, err := common.NextMinorVersion(latest)
			if err != nil {
				return "", fmt.Errorf("failed to compute next version from '%s': %w", latest, err)
			}
			log.Info(fmt.Sprintf("No version specified. Auto-incrementing to %s", next))
			return next, nil
		}
		fmt.Printf("No version specified in SKILL.md or --version flag.\n")
		fmt.Printf("Existing versions: %v  (latest: %s)\n", versionStrs, latest)
	} else {
		if pc.quiet {
			log.Info("No version specified and no existing versions found. Defaulting to 0.1.0")
			return "0.1.0", nil
		}
		fmt.Printf("No version specified in SKILL.md or --version flag.\n")
		fmt.Printf("No existing versions found for skill '%s'.\n", slug)
	}

	fmt.Print("Enter version to publish: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	newVersion := strings.TrimSpace(input)
	if newVersion == "" {
		return "", fmt.Errorf("no version provided, aborting")
	}
	if strings.Contains(newVersion, "..") || strings.ContainsAny(newVersion, "/\\") {
		return "", fmt.Errorf("invalid version '%s': contains path traversal characters", newVersion)
	}
	return newVersion, nil
}

// resolveVersionCollision checks whether the given version already exists in Artifactory.
// In interactive mode it lets the user pick: overwrite, enter a new version, or abort.
// In quiet/CI mode it fails hard so pipelines don't silently overwrite artifacts.
func (pc *PublishCommand) resolveVersionCollision(slug, version string) (string, error) {
	exists, err := common.VersionExists(pc.serverDetails, pc.repoKey, slug, version)
	if err != nil {
		log.Debug("Could not check version existence:", err.Error())
		return version, nil
	}
	if !exists {
		return version, nil
	}

	if pc.quiet {
		return "", fmt.Errorf("version %s of skill '%s' already exists. Use a different version or remove the existing one", version, slug)
	}

	log.Warn(fmt.Sprintf("Version %s of skill '%s' already exists in repository '%s'.", version, slug, pc.repoKey))
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
		if strings.Contains(newVersion, "..") || strings.ContainsAny(newVersion, "/\\") {
			return "", fmt.Errorf("invalid version '%s': contains path traversal characters", newVersion)
		}
		if err := ValidateVersion(newVersion); err != nil {
			return "", err
		}
		return pc.resolveVersionCollision(slug, newVersion)
	default:
		return "", fmt.Errorf("publish aborted by user")
	}
}

func (pc *PublishCommand) resolveZip(slug, version string) (string, error) {
	if strings.Contains(version, "..") || strings.ContainsAny(version, "/\\") {
		return "", fmt.Errorf("invalid version '%s': contains path traversal characters", version)
	}
	prebuilt := filepath.Clean(filepath.Join(pc.skillDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version)))
	if _, err := os.Stat(prebuilt); err == nil {
		log.Info("Using pre-built zip:", prebuilt)
		return prebuilt, nil
	}

	return zipSkillFolder(pc.skillDir, slug, version)
}

func isPrebuiltZip(skillDir, slug, version string) bool {
	prebuilt := filepath.Join(skillDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version))
	_, err := os.Stat(prebuilt)
	return err == nil
}

// zipEpoch is the earliest valid timestamp in ZIP format (MS-DOS epoch).
// Used as a fallback when all file mtimes are zero.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type skillFile struct {
	relPath string
	mode    os.FileMode
}

// collectFiles walks the skill directory and returns a sorted list of included
// files with their permissions, plus the max mtime across all included files.
// Sorting ensures deterministic zip output regardless of filesystem traversal order.
// The max mtime is used as a uniform timestamp for all zip entries.
func collectFiles(skillDir string) (files []skillFile, maxMtime time.Time, err error) {
	err = filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		if shouldExclude(relPath, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			files = append(files, skillFile{relPath: relPath, mode: info.Mode()})
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

// addFileToZip writes a single file into the zip writer with a deterministic header.
// Timestamps are set to uniformTime (for both modern and legacy MS-DOS fields),
// Extra field is stripped to remove platform-specific metadata, and file permissions
// are preserved from the mode captured during collection (no second os.Stat).
func addFileToZip(w *zip.Writer, skillDir string, sf skillFile, uniformTime time.Time) error {
	absPath := filepath.Join(skillDir, sf.relPath)

	header := &zip.FileHeader{
		Name:     sf.relPath,
		Method:   zip.Deflate,
		Modified: uniformTime,
	}
	header.SetModTime(uniformTime) //nolint:staticcheck // sets legacy MS-DOS ModifiedDate/ModifiedTime fields
	header.SetMode(normalizeFileMode(sf.mode))
	header.Extra = nil

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	// #nosec G304 -- absPath is from user-provided skill directory joined with a walked relative path
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(writer, file)
	return err
}

// normalizeFileMode returns a consistent Unix file mode for zip entry headers.
// On Windows, os.Stat returns 0666 for all files (no execute bit support), so
// we default to 0644 for regular files. On Unix, the real mode is preserved.
func normalizeFileMode(mode os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		return 0644
	}
	return mode
}

func zipSkillFolder(skillDir, slug, version string) (zipPath string, err error) {
	if strings.Contains(version, "..") || strings.ContainsAny(version, "/\\") {
		return "", fmt.Errorf("invalid version '%s': contains path traversal characters", version)
	}

	// Collect and sort file paths for deterministic zip output.
	// The max mtime is used as a uniform timestamp for all zip entries so that
	// the zip is byte-identical when rebuilt with the same content and mtimes.
	files, maxMtime, err := collectFiles(skillDir)
	if err != nil {
		return "", fmt.Errorf("failed to collect skill files: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files found in skill directory %s (all files may have been excluded)", skillDir)
	}
	// Guard against zero mtime (e.g. files with epoch timestamps) which produces
	// invalid MS-DOS dates before the ZIP format's 1980-01-01 minimum.
	if maxMtime.IsZero() {
		maxMtime = zipEpoch
	}

	tmpDir, err := os.MkdirTemp("", "skill-publish-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	zipPath = filepath.Clean(filepath.Join(tmpDir, fmt.Sprintf("%s-%s.zip", slug, version)))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() {
		_ = zipFile.Close()
	}()

	w := zip.NewWriter(zipFile)
	defer func() {
		if cerr := w.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to finalize zip: %w", cerr)
		}
	}()

	for _, sf := range files {
		if err = addFileToZip(w, skillDir, sf, maxMtime); err != nil {
			return "", fmt.Errorf("failed to add %s to zip: %w", sf.relPath, err)
		}
	}

	return
}

func shouldExclude(relPath string, info os.FileInfo) bool {
	name := info.Name()

	if zipExcludes[name] {
		return true
	}
	if strings.HasSuffix(name, ".pyc") {
		return true
	}
	if relPath == "." {
		return false
	}
	return false
}

func computeSHA256(path string) (string, error) {
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("invalid path: contains traversal sequence")
	}
	cleanPath := filepath.Clean(path)
	f, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (pc *PublishCommand) upload(zipPath, target string) error {
	serviceManager, err := utils.CreateUploadServiceManager(pc.serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return err
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = zipPath
	uploadParams.Target = target
	uploadParams.Flat = true

	_, _, err = serviceManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	return err
}

func (pc *PublishCommand) attachEvidence(slug, version, sha256Hex string) {
	// Flags take precedence over environment variables
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

	tmpDir, err := os.MkdirTemp("", "skill-evidence-*")
	if err != nil {
		log.Warn("Failed to create temp dir for evidence:", err.Error())
		return
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	predicatePath, err := GeneratePredicateFile(tmpDir, slug, version)
	if err != nil {
		log.Warn("Failed to generate predicate:", err.Error())
		return
	}

	markdownPath, err := GenerateMarkdownFile(tmpDir, slug, version)
	if err != nil {
		log.Warn("Failed to generate attestation markdown:", err.Error())
		return
	}

	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", pc.repoKey, slug, version, slug, version)

	opts := common.CreateEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
		SubjectSHA256:   sha256Hex,
		PredicatePath:   predicatePath,
		PredicateType:   predicateTypePublishAttestation,
		MarkdownPath:    markdownPath,
		KeyPath:         keyPath,
		KeyAlias:        alias,
	}

	// Suppress the evidence library's internal error/warn logs during this call.
	// On 403 (license issue), they are noise — we handle the error ourselves below.
	err = withSuppressedLogs(func() error {
		return common.CreateEvidence(pc.serverDetails, opts)
	})
	if err != nil {
		if isEvidenceLicenseError(err) {
			log.Info("Evidence not attached: evidence requires an Enterprise+ license. Skill upload succeeded.")
		} else {
			log.Warn("Evidence creation failed (skill upload succeeded):", err.Error())
		}
		return
	}

	log.Info("Evidence successfully attached.")
}

// withSuppressedLogs temporarily mutes all log output while fn executes,
// then restores the previous log level. Used to suppress noisy internal
// library logs when we handle errors ourselves.
func withSuppressedLogs(fn func() error) error {
	if jfLogger, ok := log.GetLogger().(*log.JfrogLogger); ok {
		prev := jfLogger.GetLogLevel()
		jfLogger.SetLogLevel(-1)
		defer jfLogger.SetLogLevel(prev)
	}
	return fn()
}

// isEvidenceLicenseError returns true when the error indicates the Artifactory
// instance does not have the license required for evidence (E+).
func isEvidenceLicenseError(err error) bool {
	return strings.Contains(err.Error(), evidenceLicenseErrFragment)
}

// RunPublish is the CLI action for `jf skills publish`.
func RunPublish(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills publish <path-to-skill-folder> [--repo <repo>] [options]")
	}

	skillDir := c.GetArgumentAt(0)
	absDir, err := filepath.Abs(skillDir)
	if err != nil {
		return fmt.Errorf("invalid skill path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("skill path '%s' is not a valid directory", skillDir)
	}

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := common.IsQuiet(c)
	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	cmd := NewPublishCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSkillDir(absDir).
		SetVersion(c.GetStringFlagValue("version")).
		SetSigningKey(c.GetStringFlagValue("signing-key")).
		SetKeyAlias(c.GetStringFlagValue("key-alias")).
		SetQuiet(quiet)

	return cmd.Run()
}
