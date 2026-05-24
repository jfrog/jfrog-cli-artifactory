package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// envDisableQuietFailure opts out of the quiet/CI failure when evidence verification fails.
// When set to a truthy value the install proceeds without verified evidence.
const envDisableQuietFailure = "JFROG_AGENT_PLUGINS_DISABLE_QUIET_FAILURE"

// summaryLabel is the noun used in summary headings.
const summaryLabel = "plugin"

// InstallCommand installs an agent plugin for configured agents or to an explicit --path.
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agents        []agentcommon.AgentSpec
	scope         agentcommon.ScopeMode
	projectDir    string
	// installPath is the base directory for `--path`. The plugin is installed at
	// <installPath>/<slug> and takes precedence over --agent / --project-dir / --global.
	installPath string
	format      string
	quiet       bool
}

func NewInstallCommand() *InstallCommand {
	return &InstallCommand{scope: agentcommon.ScopeProject}
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

func (ic *InstallCommand) SetAgents(agents []agentcommon.AgentSpec) *InstallCommand {
	ic.agents = agents
	return ic
}

// SetGlobal toggles between global and project scope.
func (ic *InstallCommand) SetGlobal(isGlobal bool) *InstallCommand {
	if isGlobal {
		ic.scope = agentcommon.ScopeGlobal
	} else {
		ic.scope = agentcommon.ScopeProject
	}
	return ic
}

// SetProjectDir sets the absolute project root used to resolve project-scope destinations.
func (ic *InstallCommand) SetProjectDir(projectRoot string) *InstallCommand {
	ic.projectDir = projectRoot
	return ic
}

func (ic *InstallCommand) SetQuiet(quiet bool) *InstallCommand {
	ic.quiet = quiet
	return ic
}

// SetFormat selects the summary output: "table" (default) or "json".
func (ic *InstallCommand) SetFormat(format string) *InstallCommand {
	ic.format = format
	return ic
}

// SetInstallPath sets the direct install base: the plugin lands at <installPath>/<slug>.
func (ic *InstallCommand) SetInstallPath(installPath string) *InstallCommand {
	ic.installPath = installPath
	return ic
}

func (ic *InstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return ic.serverDetails, nil
}

func (ic *InstallCommand) CommandName() string {
	return "agent_plugins_install"
}

func (ic *InstallCommand) Run() error {
	if ic.installPath == "" && len(ic.agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	installTargets, err := ic.resolveInstallTargets()
	if err != nil {
		return err
	}

	resolvedVersion, err := agentcommon.ResolvePackageVersion(ic.serverDetails, ic.repoKey, ic.slug, ic.version, ic.quiet)
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if ic.installPath != "" {
		log.Info(fmt.Sprintf("Installing plugin '%s' version '%s' to %s", ic.slug, ic.version, installTargets[0].DestinationDir))
	} else {
		log.Info(fmt.Sprintf("Installing plugin '%s' version '%s' for %d agent(s)", ic.slug, ic.version, len(installTargets)))
	}

	tmpDir, err := os.MkdirTemp("", "agent-plugin-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	unzipDir, err := ic.fetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	results := ic.copyExtractedToTargets(unzipDir, installTargets)

	if err := agentcommon.PrintInstallSummary(summaryLabel, ic.slug, ic.version, results, ic.format); err != nil {
		return err
	}

	for _, result := range results {
		if result.Status != agentcommon.SummaryStatusOK {
			return fmt.Errorf("installation failed for one or more agents (see summary above)")
		}
	}
	return nil
}

// fetchAndExtractTo downloads the plugin zip into tmpDir, extracts it, and runs evidence checks.
func (ic *InstallCommand) fetchAndExtractTo(tmpDir string) (string, error) {
	zipPath, err := ic.downloadZip(tmpDir)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return "", fmt.Errorf("download blocked (403): %w", err)
		}
		return "", fmt.Errorf("download failed: %w", err)
	}

	unzipDir := filepath.Join(tmpDir, "contents")
	if err := agentcommon.UnzipFile(zipPath, unzipDir); err != nil {
		return "", fmt.Errorf("unzip failed: %w", err)
	}

	if err := ic.handleEvidenceVerification(); err != nil {
		return "", err
	}
	return unzipDir, nil
}

func (ic *InstallCommand) copyExtractedToTargets(unzipDir string, targets []agentcommon.AgentTarget) []agentcommon.SummaryRow {
	results := make([]agentcommon.SummaryRow, 0, len(targets))
	for _, target := range targets {
		row := agentcommon.SummaryRow{
			Agent: target.Agent.Name,
			Scope: string(target.Scope),
			Path:  target.DestinationDir,
		}
		if err := agentcommon.EnsureDestinationDir(target.DestinationDir); err != nil {
			row.Status = agentcommon.SummaryStatusFailed
			row.Detail = err.Error()
			results = append(results, row)
			continue
		}
		if err := agentcommon.CopyDir(unzipDir, target.DestinationDir); err != nil {
			row.Status = agentcommon.SummaryStatusFailed
			row.Detail = err.Error()
			results = append(results, row)
			continue
		}
		row.Status = agentcommon.SummaryStatusOK
		row.Detail = agentcommon.SummaryDetailOKInstall
		results = append(results, row)
	}
	return results
}

func (ic *InstallCommand) handleEvidenceVerification() error {
	err := ic.verifyEvidence()
	if err == nil {
		return nil
	}
	if ic.quiet || agentcommon.IsNonInteractive() {
		if agentcommon.ShouldFailOnMissingEvidence(envDisableQuietFailure) {
			return fmt.Errorf("evidence verification failed for plugin '%s': %s. Set %s=true to proceed without evidence", ic.slug, err.Error(), envDisableQuietFailure)
		}
		log.Warn(fmt.Sprintf("Evidence verification failed for plugin '%s': %s. Proceeding with installation.", ic.slug, err.Error()))
		return nil
	}
	log.Warn("Evidence verification failed:", err.Error())
	if !coreutils.AskYesNo("The plugin is unattested. Continue with installation?", false) {
		return fmt.Errorf("installation aborted by user")
	}
	return nil
}

// resolveInstallTargets builds per-agent dest dirs, or one direct target if installPath is set.
func (ic *InstallCommand) resolveInstallTargets() ([]agentcommon.AgentTarget, error) {
	if ic.installPath != "" {
		return agentcommon.ResolveAgentTargets(ic.slug, ic.installPath, nil, "", false)
	}
	if ic.scope == agentcommon.ScopeProject && ic.projectDir == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}
	isGlobal := ic.scope == agentcommon.ScopeGlobal
	return agentcommon.ResolveAgentTargets(ic.slug, "", ic.agents, ic.projectDir, isGlobal)
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
		return "", fmt.Errorf("plugin '%s' version '%s' not found in repository '%s'", ic.slug, ic.version, ic.repoKey)
	}

	zipName := fmt.Sprintf("%s-%s.zip", ic.slug, ic.version)
	return filepath.Join(tmpDir, zipName), nil
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

// RunInstall is the CLI action for `jf agent plugins install`.
func RunInstall(commandContext *components.Context) error {
	if commandContext.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins install <slug> (--agent <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := commandContext.GetArgumentAt(0)
	if err := plugincommon.ValidateSlug(slug); err != nil {
		return err
	}

	absoluteInstallBaseDir, specs, projectDirAbs, isGlobal, err := agentcommon.ValidateInstallFlags(commandContext, plugincommon.PackageConfig())
	if err != nil {
		return err
	}

	serverDetails, err := agentcommon.GetServerDetails(commandContext)
	if err != nil {
		return err
	}
	quiet := agentcommon.IsQuiet(commandContext)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, commandContext.GetStringFlagValue("repo"), quiet, plugincommon.RepoOptions())
	if err != nil {
		return err
	}

	version := commandContext.GetStringFlagValue("version")
	format := "table"
	if commandContext.GetStringFlagValue("format") != "" {
		format = commandContext.GetStringFlagValue("format")
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
