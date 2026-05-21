package publish

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/agentcommon"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agentplugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

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

func (pc *PublishCommand) SetPluginDir(dir string) *PublishCommand {
	pc.pluginDir = dir
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

	// Update plugin.json on disk before zipping, matching jf skills publish + SKILL.md order.
	if meta.ManifestVersion != "" && meta.ManifestVersion != version {
		if err := plugincommon.UpdatePluginManifestVersions(pc.pluginDir, version); err != nil {
			return fmt.Errorf("failed to update plugin.json version: %w", err)
		}
		log.Info(fmt.Sprintf("Updated plugin.json version from '%s' to '%s'", meta.ManifestVersion, version))
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
		if sha256Hex, err = agentcommon.ComputeSHA256(zipPath); err != nil {
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
	artifactsDetailsReader, err := agentcommon.UploadPublishArtifact(pc.serverDetails, zipPath, target, collectBuildInfo, pc.buildConfiguration)
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
		if pc.quiet {
			return "", fmt.Errorf("could not verify whether version %s of plugin '%s' already exists: %w", version, slug, err)
		}
		log.Warn(fmt.Sprintf(
			"Could not verify whether version %s of plugin '%s' already exists (%v); continuing publish.",
			version, slug, err,
		))
		return version, nil
	}
	if !exists {
		return version, nil
	}

	if pc.quiet {
		return "", fmt.Errorf("version %s of plugin '%s' already exists. Use a different version or remove the existing one", version, slug)
	}

	log.Warn(fmt.Sprintf("Version %s of plugin '%s' already exists in repository '%s'.", version, slug, pc.repoKey))
	log.Info("Choose an action:")
	log.Info("  [o] Overwrite the existing version")
	log.Info("  [n] Enter a new version")
	log.Info("  [a] Abort")
	log.Info("Your choice (o/n/a): ")

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
		log.Info("Enter new version: ")
		newInput, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read user input: %w", err)
		}
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
	if agentcommon.IsPrebuiltPublishZip(pc.pluginDir, slug, version) {
		prebuiltPath := agentcommon.PrebuiltPublishZipPath(pc.pluginDir, slug, version)
		log.Info("Using pre-built zip:", prebuiltPath)
		return prebuiltPath, "", true, nil
	}

	zipPath, sha256Hex, err = agentcommon.ZipPublishBundle(agentcommon.ZipPublishOptions{
		SourceDir:      pc.pluginDir,
		Slug:           slug,
		Version:        version,
		TempDirPrefix:  "agent-plugin-publish-",
		ContentLabel:   "plugin",
		HashWhileWrite: true,
	})
	return zipPath, sha256Hex, false, err
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

	err = agentcommon.WithSuppressedLogs(func() error {
		return agentcommon.CreateEvidence(pc.serverDetails, opts)
	})
	if err != nil {
		if agentcommon.IsEvidenceLicenseError(err) {
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

// RunPublish is the CLI action for `jf ai plugins publish <path>`.
func RunPublish(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf ai plugins publish <path-to-plugin-folder> [--repo <repo>] [options]")
	}
	absDir, err := validatePluginDir(c.GetArgumentAt(0))
	if err != nil {
		return err
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
