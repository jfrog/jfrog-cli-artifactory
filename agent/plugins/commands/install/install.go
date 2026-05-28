package install

import (
	"errors"
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

// resolveLatestPluginVersion is swappable in tests.
var resolveLatestPluginVersion = plugincommon.ResolveLatestPluginVersion

// scope is project (under project root) or global (agent home paths).
type scope string

const (
	scopeProject scope = "project"
	scopeGlobal  scope = "global"
)

// InstallCommand installs an agent plugin for a single agent or a direct --path target.
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agent         plugincommon.AgentSpec // singular: plugins install accepts only one harness
	scope         scope
	projectDir    string // project root for project scope (--project-dir)
	// installPath is the base directory for jf agent plugins install --path. The plugin is installed at
	// <installPath>/<slug> and takes precedence over --harness / --project-dir / --global.
	installPath string
	format      string
	quiet       bool
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

func (ic *InstallCommand) SetAgent(agent plugincommon.AgentSpec) *InstallCommand {
	ic.agent = agent
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

// SetInstallPath sets a direct install base: plugin at <base>/<slug>.
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
	if ic.installPath == "" && ic.agent.Name == "" {
		return fmt.Errorf("--harness is required unless --path is set")
	}

	target, err := ic.resolveAgentTarget()
	if err != nil {
		return err
	}

	resolvedVersion, err := ic.resolveVersion()
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if err := plugincommon.ValidateVersion(ic.version); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Installing plugin '%s' version '%s' to %s", ic.slug, ic.version, target.DestinationDir))

	tmpDir, err := os.MkdirTemp("", "plugin-install-*")
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

	results := ic.copyExtractedToTarget(unzipDir, target)

	if err := agentcommon.PrintInstallSummary("Plugin", ic.slug, ic.version, results, ic.format); err != nil {
		return err
	}

	for _, result := range results {
		if result.Status != agentcommon.SummaryStatusOK {
			return fmt.Errorf("installation failed (see summary above)")
		}
	}
	return nil
}

// resolveVersion picks the version to install based on the requested --version and --harness.
//
//   - --version 1.0.0 (exact) → use directly
//   - --version latest         → ListPluginVersions on Artifactory, pick latest semver
//   - --version "" + harness   → download <harness>-marketplace.json, look up slug, use that version,
//     then delete marketplace.json (deferred cleanup)
//   - --version "" + path-only → ListPluginVersions on Artifactory, pick latest semver (same as skills)
func (ic *InstallCommand) resolveVersion() (string, error) {
	requested := strings.TrimSpace(ic.version)
	if requested != "" && requested != "latest" {
		return requested, nil
	}
	if requested == "latest" || ic.agent.Name == "" {
		return resolveLatestPluginVersion(ic.serverDetails, ic.repoKey, ic.slug)
	}
	// Empty --version with harness: resolve from marketplace.
	version, err := plugincommon.ResolveVersionFromMarketplace(ic.serverDetails, ic.repoKey, ic.agent.Name, ic.slug)
	if err != nil {
		if errors.Is(err, plugincommon.ErrMarketplaceNotFound) {
			return "", fmt.Errorf(
				"'%s' is a supported agent, but %s is not available in '%s'. "+
					"To install from the common marketplace, re-run with --version <ver> (e.g. --version 1.0.0 or --version latest)",
				ic.agent.Name, plugincommon.MarketplaceFileName(ic.agent.Name), ic.repoKey,
			)
		}
		return "", err
	}
	return version, nil
}

func (ic *InstallCommand) fetchAndExtractTo(tmpDir string) (string, error) {
	zipPath, err := ic.downloadZip(tmpDir)
	if err != nil {
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

func (ic *InstallCommand) copyExtractedToTarget(unzipDir string, target plugincommon.AgentTarget) []agentcommon.SummaryRow {
	failure := func(err error) agentcommon.SummaryRow {
		return agentcommon.SummaryRow{
			Agent:  target.Agent.Name,
			Scope:  string(target.Scope),
			Path:   target.DestinationDir,
			Status: agentcommon.SummaryStatusFailed,
			Detail: err.Error(),
		}
	}
	if err := agentcommon.EnsureDestinationDir(target.DestinationDir); err != nil {
		return []agentcommon.SummaryRow{failure(err)}
	}
	if err := agentcommon.CopyDir(unzipDir, target.DestinationDir); err != nil {
		return []agentcommon.SummaryRow{failure(err)}
	}
	if err := ic.writePluginInfoManifest(target); err != nil {
		return []agentcommon.SummaryRow{failure(err)}
	}
	return []agentcommon.SummaryRow{{
		Agent:  target.Agent.Name,
		Scope:  string(target.Scope),
		Path:   target.DestinationDir,
		Status: agentcommon.SummaryStatusOK,
		Detail: agentcommon.SummaryDetailOKInstall,
	}}
}

func (ic *InstallCommand) handleEvidenceVerification() error {
	err := ic.verifyEvidence()
	if err == nil {
		return nil
	}
	if ic.quiet || agentcommon.IsNonInteractive() {
		log.Warn(fmt.Sprintf("Evidence verification failed for plugin '%s': %s. Proceeding with installation.", ic.slug, err.Error()))
		return nil
	}
	log.Warn("Evidence verification failed:", err.Error())
	if !coreutils.AskYesNo("The plugin is unattested. Continue with installation?", false) {
		return fmt.Errorf("installation aborted by user")
	}
	return nil
}

func (ic *InstallCommand) resolveAgentTarget() (plugincommon.AgentTarget, error) {
	if ic.installPath != "" {
		return plugincommon.ResolveAgentTarget(ic.slug, ic.installPath, plugincommon.AgentSpec{}, "", false)
	}
	if ic.scope == scopeProject && ic.projectDir == "" {
		return plugincommon.AgentTarget{}, fmt.Errorf("project directory is required for project-scoped install")
	}
	isGlobal := ic.scope == scopeGlobal
	return plugincommon.ResolveAgentTarget(ic.slug, "", ic.agent, ic.projectDir, isGlobal)
}

func (ic *InstallCommand) writePluginInfoManifest(target plugincommon.AgentTarget) error {
	manifest := plugincommon.PluginInfoManifest{
		SchemaVersion:    plugincommon.PluginInfoManifestSchemaVersion,
		Repo:             ic.repoKey,
		Slug:             ic.slug,
		InstalledVersion: ic.version,
		Scope:            string(target.Scope),
		Agent:            target.Agent.Name,
	}
	if target.Scope == plugincommon.ScopeProject && ic.projectDir != "" {
		manifest.ProjectDir = ic.projectDir
	}
	return plugincommon.WritePluginInfoManifest(target.DestinationDir, manifest)
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
func RunInstall(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins install <slug> (--harness <name> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := c.GetArgumentAt(0)
	if err := plugincommon.ValidateSlug(slug); err != nil {
		return err
	}

	absoluteInstallBaseDir, spec, projectDirAbs, isGlobal, err := plugincommon.ValidateInstallFlags(c)
	if err != nil {
		return err
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}
	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, plugincommon.RepoOptions())
	if err != nil {
		return err
	}

	version := c.GetStringFlagValue("version")
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	cmd := NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(version).
		SetFormat(format).
		SetQuiet(quiet)

	if absoluteInstallBaseDir != "" {
		return cmd.SetInstallPath(absoluteInstallBaseDir).Run()
	}

	return cmd.
		SetAgent(spec).
		SetGlobal(isGlobal).
		SetProjectDir(projectDirAbs).
		Run()
}
