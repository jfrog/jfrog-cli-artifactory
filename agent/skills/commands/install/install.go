package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// scope is project (under project root) or global (agent home paths).
type scope string

const (
	scopeProject scope = "project"
	scopeGlobal  scope = "global"
)

// InstallCommand installs a skill for configured agents or legacy --path (update).
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agents        []common.AgentSpec
	scope         scope
	projectDir    string // project root for project scope (--project-dir)
	// installPath is the base directory for jf agent skills install --path. The skill is installed at
	// <installPath>/<slug> and takes precedence over --harness / --project-dir / --global.
	installPath string
	format      string
	quiet       bool
	// explicitTargets, when set, overrides resolveAgentTargetDirectories (used by skills update).
	explicitTargets []common.AgentTarget
	suppressSummary bool
}

func NewInstallCommand() *InstallCommand {
	return &InstallCommand{scope: scopeProject}
}

func (ic *InstallCommand) SetServerDetails(details *config.ServerDetails) *InstallCommand {
	ic.serverDetails = details
	return ic
}

func (ic *InstallCommand) SetRepoKey(repoKey string) *InstallCommand {
	ic.repoKey = repoKey
	return ic
}

func (ic *InstallCommand) SetSlug(slug string) *InstallCommand {
	ic.slug = slug
	return ic
}

func (ic *InstallCommand) SetVersion(version string) *InstallCommand {
	ic.version = version
	return ic
}

func (ic *InstallCommand) SetAgents(agents []common.AgentSpec) *InstallCommand {
	ic.agents = agents
	return ic
}

// SetGlobal sets global vs project scope.
func (ic *InstallCommand) SetGlobal(isGlobal bool) *InstallCommand {
	if isGlobal {
		ic.scope = scopeGlobal
	} else {
		ic.scope = scopeProject
	}
	return ic
}

// SetProjectDir sets absolute project root for project scope.
func (ic *InstallCommand) SetProjectDir(projectRoot string) *InstallCommand {
	ic.projectDir = projectRoot
	return ic
}

func (ic *InstallCommand) SetQuiet(quiet bool) *InstallCommand {
	ic.quiet = quiet
	return ic
}

// SetFormat sets summary output: "table" (default) or "json".
func (ic *InstallCommand) SetFormat(format string) *InstallCommand {
	ic.format = format
	return ic
}

// SetInstallPath sets direct install base (same as skills update --path): skill at <base>/<slug>.
func (ic *InstallCommand) SetInstallPath(installPath string) *InstallCommand {
	ic.installPath = installPath
	return ic
}

// SetTargets overrides target resolution. Used by skills update for a filtered subset.
func (ic *InstallCommand) SetTargets(targets []common.AgentTarget) *InstallCommand {
	ic.explicitTargets = targets
	return ic
}

// SetSuppressSummary skips PrintSummary at the end of Run (caller prints a merged summary).
func (ic *InstallCommand) SetSuppressSummary(suppress bool) *InstallCommand {
	ic.suppressSummary = suppress
	return ic
}

func (ic *InstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return ic.serverDetails, nil
}

func (ic *InstallCommand) CommandName() string {
	return "skills_install"
}

func (ic *InstallCommand) Run() error {
	if ic.installPath == "" && len(ic.agents) == 0 && len(ic.explicitTargets) == 0 {
		return fmt.Errorf("--harness is required")
	}

	installTargets, err := ic.resolveAgentTargetDirectories()
	if err != nil {
		return err
	}

	resolvedVersion, err := common.ResolveSkillVersion(ic.serverDetails, ic.repoKey, ic.slug, ic.version, ic.quiet)
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if ic.installPath != "" {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' to %s", ic.slug, ic.version, installTargets[0].DestinationDir))
	} else {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' for %d harness(es)", ic.slug, ic.version, len(installTargets)))
	}

	tmpDir, err := os.MkdirTemp("", "skill-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	unzipDir, err := ic.FetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	results := ic.CopyExtractedToTargets(unzipDir, installTargets)

	if !ic.suppressSummary {
		if err := agentcommon.PrintInstallSummary("Skill", ic.slug, ic.version, results, ic.format); err != nil {
			return err
		}
	}

	for _, result := range results {
		if result.Status != agentcommon.SummaryStatusOK {
			if ic.suppressSummary {
				return fmt.Errorf("installation failed for one or more targets")
			}
			return fmt.Errorf("installation failed for one or more agents (see summary above)")
		}
	}
	return nil
}

// FetchAndExtractTo downloads the skill zip into tmpDir, extracts it, and runs evidence checks.
// The returned unzipDir is under tmpDir; callers must keep tmpDir until copies finish.
func (ic *InstallCommand) FetchAndExtractTo(tmpDir string) (unzipDir string, err error) {
	zipPath, err := ic.downloadZip(tmpDir)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return "", ic.diagnoseDownloadForbidden(err)
		}
		return "", fmt.Errorf("download failed: %w", err)
	}

	unzipDir = filepath.Join(tmpDir, "contents")
	if err := agentcommon.UnzipFile(zipPath, unzipDir); err != nil {
		return "", fmt.Errorf("unzip failed: %w", err)
	}

	if err := ic.handleEvidenceVerification(); err != nil {
		return "", err
	}
	return unzipDir, nil
}

// CopyExtractedToTargets copies an unpacked skill tree to the given resolved targets and
// writes a skill-info manifest per target.
func (ic *InstallCommand) CopyExtractedToTargets(unzipDir string, installTargets []common.AgentTarget) []agentcommon.SummaryRow {
	results := make([]agentcommon.SummaryRow, 0, len(installTargets))
	for _, target := range installTargets {
		if err := agentcommon.EnsureDestinationDir(target.DestinationDir); err != nil {
			results = append(results, failureRow(target, err))
			continue
		}
		if err := agentcommon.CopyDir(unzipDir, target.DestinationDir); err != nil {
			results = append(results, failureRow(target, err))
			continue
		}
		if err := ic.writeSkillInfoManifest(target); err != nil {
			results = append(results, failureRow(target, err))
			continue
		}
		results = append(results, agentcommon.SummaryRow{
			Agent:  target.Agent.Name,
			Scope:  string(target.Scope),
			Path:   target.DestinationDir,
			Status: agentcommon.SummaryStatusOK,
			Detail: agentcommon.SummaryDetailOKInstall,
		})
	}
	return results
}

func failureRow(target common.AgentTarget, err error) agentcommon.SummaryRow {
	return agentcommon.SummaryRow{
		Agent:  target.Agent.Name,
		Scope:  string(target.Scope),
		Path:   target.DestinationDir,
		Status: agentcommon.SummaryStatusFailed,
		Detail: err.Error(),
	}
}

func (ic *InstallCommand) handleEvidenceVerification() error {
	err := ic.verifyEvidence()
	if err == nil {
		return nil
	}
	if ic.quiet || agentcommon.IsNonInteractive() {
		if common.ShouldFailOnMissingEvidence() {
			return fmt.Errorf("evidence verification failed for skill '%s': %s. Set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true to proceed without evidence", ic.slug, err.Error())
		}
		log.Warn(fmt.Sprintf("Evidence verification failed for skill '%s': %s. Proceeding with installation.", ic.slug, err.Error()))
		return nil
	}
	log.Warn("Evidence verification failed:", err.Error())
	if !coreutils.AskYesNo("The skill is unattested. Continue with installation?", false) {
		return fmt.Errorf("installation aborted by user")
	}
	return nil
}

// resolveAgentTargetDirectories builds per-agent dest dirs, or one direct target if installPath is set (install/update --path).
func (ic *InstallCommand) resolveAgentTargetDirectories() ([]common.AgentTarget, error) {
	if len(ic.explicitTargets) > 0 {
		return ic.explicitTargets, nil
	}
	if ic.installPath != "" {
		return common.ResolveAgentTargets(ic.slug, ic.installPath, nil, "", false)
	}
	if ic.scope == scopeProject && ic.projectDir == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}
	isGlobal := ic.scope == scopeGlobal
	return common.ResolveAgentTargets(ic.slug, "", ic.agents, ic.projectDir, isGlobal)
}

func (ic *InstallCommand) writeSkillInfoManifest(target common.AgentTarget) error {
	dirName := filepath.Base(target.DestinationDir)
	slug := ic.slug
	if dirName != "" && dirName != slug {
		log.Warn(fmt.Sprintf("Install directory name %q differs from slug %q; manifest will record slug %q for API consistency", dirName, slug, slug))
	}
	manifest := common.SkillInfoManifest{
		SchemaVersion:    common.SkillInfoManifestSchemaVersion,
		Repo:             ic.repoKey,
		Slug:             slug,
		InstalledVersion: ic.version,
		Scope:            string(target.Scope),
		Agent:            target.Agent.Name,
	}
	if target.Scope == common.ScopeProject && ic.projectDir != "" {
		manifest.ProjectDir = ic.projectDir
	}
	return common.WriteSkillInfoManifest(target.DestinationDir, manifest)
}

func (ic *InstallCommand) downloadZip(tmpDir string) (string, error) {
	serviceManager, err := utils.CreateDownloadServiceManager(ic.serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return "", err
	}

	pattern := fmt.Sprintf("%s/%s/%s/%s-%s.zip", ic.repoKey, ic.slug, ic.version, ic.slug, ic.version)

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = pattern
	downloadParams.Target = tmpDir + "/"
	downloadParams.Flat = true

	totalDownloaded, totalFailed, err := serviceManager.DownloadFiles(downloadParams)
	if err != nil {
		return "", err
	}
	if totalFailed > 0 {
		return "", fmt.Errorf("download failed for %s", pattern)
	}
	if totalDownloaded == 0 {
		return "", fmt.Errorf("skill '%s' version '%s' not found in repository '%s'", ic.slug, ic.version, ic.repoKey)
	}

	zipName := fmt.Sprintf("%s-%s.zip", ic.slug, ic.version)
	zipPath := filepath.Join(tmpDir, zipName)
	return zipPath, nil
}

// diagnoseDownloadForbidden checks the Xray status API when a download returns 403.
// If the artifact is blocked by Xray, it returns a specific error message.
// Otherwise, it returns the original error.
func (ic *InstallCommand) diagnoseDownloadForbidden(originalErr error) error {
	artifactPath := fmt.Sprintf("%s/%s/%s-%s.zip", ic.slug, ic.version, ic.slug, ic.version)
	sm, err := utils.CreateServiceManager(ic.serverDetails, 3, 0, false)
	if err != nil {
		return fmt.Errorf("download blocked (403): %w", originalErr)
	}
	resp, err := sm.GetSkillXrayStatus(ic.repoKey, artifactPath)
	if err != nil {
		return fmt.Errorf("download blocked (403): %w", originalErr)
	}
	if resp.Status == services.SkillXrayStatusBlocked {
		return fmt.Errorf("skill '%s' v%s is blocked by Xray security scan. The artifact has security or license violations and cannot be downloaded", ic.slug, ic.version)
	}
	return fmt.Errorf("download failed: %w", originalErr)
}

func (ic *InstallCommand) verifyEvidence() error {
	if ic.repoKey == "" || ic.slug == "" || ic.version == "" {
		return fmt.Errorf("cannot verify evidence: repoKey, slug, and version must all be set")
	}

	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", ic.repoKey, ic.slug, ic.version, ic.slug, ic.version)

	return agentcommon.VerifyEvidence(ic.serverDetails, agentcommon.VerifyEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
	})
}

// RunInstall is the CLI action for `jf agent skills install`.
func RunInstall(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent skills install <slug> (--harness <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := c.GetArgumentAt(0)
	if err := publish.ValidateSlug(slug); err != nil {
		return err
	}

	absoluteInstallBaseDir, specs, projectDirAbs, isGlobal, err := common.ValidateInstallFlags(c)
	if err != nil {
		return err
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

	version := c.GetStringFlagValue("version")
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}
	if absoluteInstallBaseDir != "" {
		return NewInstallCommand().
			SetServerDetails(serverDetails).
			SetRepoKey(repoKey).
			SetSlug(slug).
			SetVersion(version).
			SetInstallPath(absoluteInstallBaseDir).
			SetFormat(format).
			SetQuiet(quiet).
			Run()
	}

	return NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(version).
		SetAgents(specs).
		SetGlobal(isGlobal).
		SetProjectDir(projectDirAbs).
		SetFormat(format).
		SetQuiet(quiet).
		Run()
}
