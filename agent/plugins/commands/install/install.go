package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// resolvePluginVersion is swappable in tests.
var resolvePluginVersion = plugincommon.ResolvePluginVersion

// InstallCommand installs an agent plugin for configured agents or a direct --path target.
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agents        []plugincommon.AgentSpec
	scope         agentcommon.InstallScope
	projectDir    string // project root for project scope (--project-dir)
	// installPath is the base directory for jf agent plugins install --path. The plugin is installed at
	// <installPath>/<slug> and takes precedence over --harness / --project-dir / --global.
	installPath string
	format      string
	quiet       bool
}

func NewInstallCommand() *InstallCommand {
	return &InstallCommand{scope: agentcommon.InstallScopeProject}
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

func (ic *InstallCommand) SetAgents(agents []plugincommon.AgentSpec) *InstallCommand {
	ic.agents = agents
	return ic
}

// SetGlobal sets global vs project scope.
func (ic *InstallCommand) SetGlobal(isGlobal bool) *InstallCommand {
	if isGlobal {
		ic.scope = agentcommon.InstallScopeGlobal
	} else {
		ic.scope = agentcommon.InstallScopeProject
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
	if ic.installPath == "" && len(ic.agents) == 0 {
		return fmt.Errorf("--harness is required unless --path is set")
	}

	installTargets, err := ic.resolveAgentTargetDirectories()
	if err != nil {
		return err
	}

	resolvedVersion, err := ic.resolveVersion()
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if err := agentcommon.ValidateSemver(ic.version); err != nil {
		return err
	}

	if ic.installPath != "" {
		log.Info(fmt.Sprintf("Installing plugin '%s' version '%s' to %s", ic.slug, ic.version, installTargets[0].DestinationDir))
	} else {
		log.Info(fmt.Sprintf("Installing plugin '%s' version '%s' for %d harness(es)", ic.slug, ic.version, len(installTargets)))
	}

	tmpDir, err := os.MkdirTemp("", "plugin-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		// Best-effort cleanup of install temp dir.
		_ = os.RemoveAll(tmpDir)
	}()

	unzipDir, err := ic.FetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	results := ic.CopyExtractedToTargets(unzipDir, installTargets)

	if err := agentcommon.PrintInstallSummary("Plugin", ic.slug, ic.version, results, ic.format); err != nil {
		return err
	}

	for _, result := range results {
		if result.Status != agentcommon.SummaryStatusOK {
			return fmt.Errorf("installation failed for one or more agents (see summary above)")
		}
	}
	return nil
}

// resolveVersion picks the version to install based on the requested --version and --harness.
//
//   - --version 1.0.0 (exact) → verify in repository; prompt to pick if missing (interactive)
//   - --version latest         → ListPluginVersions on Artifactory, pick latest semver
//   - --version "" + harness   → download <harness>-marketplace.json, look up slug, use that version,
//     then delete marketplace.json (deferred cleanup)
//   - --version "" + path-only → ListPluginVersions on Artifactory, pick latest semver (same as skills)
func (ic *InstallCommand) resolveVersion() (string, error) {
	requested := strings.TrimSpace(ic.version)
	if requested == "" && len(ic.agents) > 0 {
		return ic.resolveVersionFromMarketplaces()
	}
	return resolvePluginVersion(ic.serverDetails, ic.repoKey, ic.slug, requested, ic.quiet)
}

func (ic *InstallCommand) resolveVersionFromMarketplaces() (string, error) {
	var resolved string
	for _, agent := range ic.agents {
		version, err := plugincommon.ResolveVersionFromMarketplace(ic.serverDetails, ic.repoKey, agent.Name, ic.slug)
		if err != nil {
			if errors.Is(err, plugincommon.ErrMarketplaceNotFound) {
				return "", fmt.Errorf(
					"'%s' is a supported agent, but %s is not available in repository '%s'; %s",
					agent.Name, plugincommon.MarketplaceFileName(agent.Name), ic.repoKey, plugincommon.InstallBypassMarketplaceHint,
				)
			}
			return "", err
		}
		if resolved == "" {
			resolved = version
			continue
		}
		if version != resolved {
			return "", fmt.Errorf(
				"marketplace versions differ across harnesses for plugin '%s' (%s for %s vs %s for %s); specify --version explicitly",
				ic.slug, resolved, ic.agents[0].Name, version, agent.Name,
			)
		}
	}
	return resolved, nil
}

// FetchAndExtractTo downloads the plugin zip into tmpDir, extracts it, and runs evidence checks.
// The returned unzipDir is under tmpDir; callers must keep tmpDir until copies finish.
func (ic *InstallCommand) FetchAndExtractTo(tmpDir string) (string, error) {
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

// CopyExtractedToTargets copies an unpacked plugin tree to the given resolved targets and
// writes a plugin-info manifest per target.
func (ic *InstallCommand) CopyExtractedToTargets(unzipDir string, installTargets []plugincommon.AgentTarget) []agentcommon.SummaryRow {
	results := make([]agentcommon.SummaryRow, 0, len(installTargets))
	for _, target := range installTargets {
		if err := agentcommon.EnsureDestinationDir(target.DestinationDir); err != nil {
			results = append(results, agentcommon.InstallFailureRow(target.Agent.Name, string(target.Scope), target.DestinationDir, err))
			continue
		}
		if err := agentcommon.CopyDir(unzipDir, target.DestinationDir); err != nil {
			results = append(results, agentcommon.InstallFailureRow(target.Agent.Name, string(target.Scope), target.DestinationDir, err))
			continue
		}
		if err := ic.writePluginInfoManifest(target); err != nil {
			results = append(results, agentcommon.InstallFailureRow(target.Agent.Name, string(target.Scope), target.DestinationDir, err))
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

func (ic *InstallCommand) handleEvidenceVerification() error {
	err := ic.verifyEvidence()
	if err == nil {
		return nil
	}
	if ic.quiet || agentcommon.IsNonInteractive() {
		if agentcommon.ShouldFailOnMissingEvidenceForPlugins() {
			return fmt.Errorf("evidence verification failed for plugin '%s': %s. %s", ic.slug, err.Error(), agentcommon.DisableQuietFailureEvidenceHintForPlugins())
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

func (ic *InstallCommand) resolveAgentTargetDirectories() ([]plugincommon.AgentTarget, error) {
	if ic.installPath != "" {
		// projectDirAbs is "" because --path mode uses an absolute path directly
		// e.g., jf agent plugins install web --path /home/user/plugins
		return agentcommon.ResolveAgentTargets(ic.slug, ic.installPath, nil, "", false)
	}
	if ic.scope == agentcommon.InstallScopeProject && ic.projectDir == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}
	isGlobal := ic.scope == agentcommon.InstallScopeGlobal
	// Path is "" because harness mode uses project or global scope
	// e.g., jf agent plugins install web --harness claude --global
	return agentcommon.ResolveAgentTargets(ic.slug, "", ic.agents, ic.projectDir, isGlobal)
}

func (ic *InstallCommand) writePluginInfoManifest(target plugincommon.AgentTarget) error {
	manifest := agentcommon.InstallInfoManifest{
		SchemaVersion:    agentcommon.InstallInfoManifestSchemaVersion,
		Repo:             ic.repoKey,
		Slug:             ic.slug,
		InstalledVersion: ic.version,
		Scope:            string(target.Scope),
		Agent:            target.Agent.Name,
	}
	if target.Scope == plugincommon.ScopeProject && ic.projectDir != "" {
		manifest.ProjectDir = ic.projectDir
	}
	return agentcommon.WriteInstallInfoManifest(target.DestinationDir, plugincommon.PluginInfoManifestFile, manifest)
}

func (ic *InstallCommand) downloadZip(tmpDir string) (string, error) {
	return agentcommon.DownloadPackageZip(ic.serverDetails, ic.repoKey, ic.slug, ic.version, tmpDir, "plugin")
}

func (ic *InstallCommand) verifyEvidence() error {
	return agentcommon.VerifyPackageEvidence(ic.serverDetails, ic.repoKey, ic.slug, ic.version)
}

// RunInstall is the CLI action for `jf agent plugins install`.
func RunInstall(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins install <slug> (--harness <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := c.GetArgumentAt(0)
	if err := agentcommon.ValidateSlug(slug); err != nil {
		return err
	}

	flags, err := agentcommon.ValidateInstallFlags(c, plugincommon.Agents, agentcommon.PluginsAgentsKey, plugincommon.RegistryHelp)
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

	if flags.PathMode() {
		return cmd.SetInstallPath(flags.AbsoluteInstallBaseDir).Run()
	}

	return cmd.
		SetAgents(flags.Specs).
		SetGlobal(flags.IsGlobal).
		SetProjectDir(flags.ProjectDirAbs).
		Run()
}
