package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
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

type PublishCommand struct {
	serverDetails       *config.ServerDetails
	repoKey             string
	skillDir            string
	version             string
	signingKey          string
	keyAlias            string
	quiet               bool
	skipScan            bool
	autoDeleteOnFailure bool
	buildConfiguration  *build.BuildConfiguration
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

func (pc *PublishCommand) SetSkipScan(skip bool) *PublishCommand {
	pc.skipScan = skip
	return pc
}

func (pc *PublishCommand) SetAutoDeleteOnFailure(autoDelete bool) *PublishCommand {
	pc.autoDeleteOnFailure = autoDelete
	return pc
}

func (pc *PublishCommand) SetBuildConfiguration(buildConfig *build.BuildConfiguration) *PublishCommand {
	pc.buildConfiguration = buildConfig
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
	if err := agentcommon.ValidateSlug(slug); err != nil {
		return err
	}

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

	if err := agentcommon.ValidateSemver(version); err != nil {
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

	zipPath, zipTmpDir, sha256Hex, err := pc.resolveZip(slug, version)
	// The temp dir is created before the archive is written, so it can exist even
	// when the write fails. Register cleanup before checking err.
	defer func() {
		if zipTmpDir != "" {
			_ = os.RemoveAll(zipTmpDir) // best-effort temp cleanup after upload
		}
	}()
	if err != nil {
		return err
	}
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
	// The upload is flat, so the artifact keeps the zip's base name. Prebuilt zips
	// use a different name than generated ones, so derive it instead of rebuilding it.
	zipName := filepath.Base(zipPath)
	pc.attachEvidence(slug, version, sha256Hex, fmt.Sprintf("%s/%s/%s/%s", pc.repoKey, slug, version, zipName))

	// Post-publish Xray scan gate check
	artifactPath := fmt.Sprintf("%s/%s/%s", slug, version, zipName)
	if err := common.CheckXrayGate(common.XrayGateParams{
		ServerDetails:       pc.serverDetails,
		RepoKey:             pc.repoKey,
		ArtifactPath:        artifactPath,
		Slug:                slug,
		Version:             version,
		SkipScan:            pc.skipScan,
		AutoDeleteOnFailure: pc.autoDeleteOnFailure,
		Quiet:               pc.quiet,
	}); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Skill '%s' version '%s' published successfully.", slug, version))
	return nil
}

// resolveMissingVersion handles the case where neither --version nor SKILL.md frontmatter
// provides a version. It delegates to the common version resolver and adds skills-specific validation.
func (pc *PublishCommand) resolveMissingVersion(slug string) (string, error) {
	newVersion, err := agentcommon.ResolveMissingVersion(agentcommon.ResolveMissingVersionOpts{
		ServerDetails: pc.serverDetails,
		RepoKey:       pc.repoKey,
		Slug:          slug,
		Quiet:         pc.quiet,
		ListVersions: func(sd *config.ServerDetails, repo, s string) ([]agentcommon.PublishableVersion, error) {
			versions, err := common.ListVersions(sd, repo, s)
			if err != nil {
				return nil, err
			}
			// Convert SkillVersion to PublishableVersion
			result := make([]agentcommon.PublishableVersion, len(versions))
			for i, v := range versions {
				result[i] = agentcommon.PublishableVersion{Version: v.Version}
			}
			return result, nil
		},
	})
	if err != nil {
		return "", err
	}

	// Skills-specific validation for path traversal
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

	input, err := agentcommon.PromptLine("Your choice (o/n/a): ")
	if err != nil {
		return "", err
	}
	choice := strings.ToLower(input)

	switch choice {
	case "o":
		log.Info(fmt.Sprintf("Overwriting version %s...", version))
		return version, nil
	case "n":
		newVersion, err := agentcommon.PromptLine("Enter new version: ")
		if err != nil {
			return "", err
		}
		if newVersion == "" {
			return "", fmt.Errorf("no version provided, aborting")
		}
		if err := agentcommon.ValidateSemver(newVersion); err != nil {
			return "", err
		}
		return pc.resolveVersionCollision(slug, newVersion)
	default:
		return "", fmt.Errorf("publish aborted by user")
	}
}

// resolveZip locates or builds the publish zip and, when it was built locally,
// also returns the temp directory holding the zip and its SHA256 (computed in the
// same pass as the write). zipTmpDir is empty for prebuilt zips; callers should
// defer os.RemoveAll(zipTmpDir) when non-empty, including on error, because the
// temp directory is created before the archive is written. The result order mirrors
// agentcommon.ZipPublishBundle so the values cannot be transposed on their way out.
func (pc *PublishCommand) resolveZip(slug, version string) (zipPath, zipTmpDir, sha256Hex string, err error) {
	if agentcommon.IsPrebuiltPublishZip(pc.skillDir, slug, version) {
		prebuiltPath := agentcommon.PrebuiltPublishZipPath(pc.skillDir, slug, version)
		log.Info("Using pre-built zip:", prebuiltPath)
		return prebuiltPath, "", "", nil
	}

	return agentcommon.ZipPublishBundle(agentcommon.ZipPublishOptions{
		SourceDir:      pc.skillDir,
		Slug:           slug,
		Version:        version,
		TempDirPrefix:  "skill-publish-",
		ContentLabel:   "skill",
		HashWhileWrite: true,
	})
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

	opts := agentcommon.CreateEvidenceOpts{
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
	err = agentcommon.WithSuppressedLogs(func() error {
		return agentcommon.CreateEvidence(pc.serverDetails, opts)
	})
	if err != nil {
		if agentcommon.IsEvidenceLicenseError(err) {
			log.Info("Evidence not attached: evidence requires an Enterprise+ license. Skill upload succeeded.")
		} else {
			log.Warn("Evidence creation failed (skill upload succeeded):", err.Error())
		}
		return
	}

	log.Info("Evidence successfully attached.")
}

// RunPublish is the CLI action for `jf agent skills publish`.
func RunPublish(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent skills publish <path-to-skill-folder> [--repo <repo>] [options]")
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

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, common.RepoOptions())
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
		SetSkillDir(absDir).
		SetVersion(c.GetStringFlagValue("version")).
		SetSigningKey(c.GetStringFlagValue("signing-key")).
		SetKeyAlias(c.GetStringFlagValue("key-alias")).
		SetQuiet(quiet).
		SetSkipScan(c.GetBoolFlagValue("skip-scan")).
		SetAutoDeleteOnFailure(c.GetBoolFlagValue("auto-delete-on-failure")).
		SetBuildConfiguration(buildConfig)

	return cmd.Run()
}
