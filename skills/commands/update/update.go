package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type UpdateCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	// Location strategy
	agentName  string
	projectDir string
	global     bool
	// Behaviour
	dryRun bool
	force  bool
	quiet  bool
}

func NewUpdateCommand() *UpdateCommand {
	return &UpdateCommand{}
}

func (uc *UpdateCommand) SetServerDetails(details *config.ServerDetails) *UpdateCommand {
	uc.serverDetails = details
	return uc
}

func (uc *UpdateCommand) SetRepoKey(repoKey string) *UpdateCommand {
	uc.repoKey = repoKey
	return uc
}

func (uc *UpdateCommand) SetSlug(slug string) *UpdateCommand {
	uc.slug = slug
	return uc
}

func (uc *UpdateCommand) SetVersion(version string) *UpdateCommand {
	uc.version = version
	return uc
}

func (uc *UpdateCommand) SetAgentName(agentName string) *UpdateCommand {
	uc.agentName = agentName
	return uc
}

func (uc *UpdateCommand) SetProjectDir(projectDir string) *UpdateCommand {
	uc.projectDir = projectDir
	return uc
}

func (uc *UpdateCommand) SetGlobal(global bool) *UpdateCommand {
	uc.global = global
	return uc
}

func (uc *UpdateCommand) SetDryRun(dryRun bool) *UpdateCommand {
	uc.dryRun = dryRun
	return uc
}

func (uc *UpdateCommand) SetForce(force bool) *UpdateCommand {
	uc.force = force
	return uc
}

func (uc *UpdateCommand) SetQuiet(quiet bool) *UpdateCommand {
	uc.quiet = quiet
	return uc
}

func (uc *UpdateCommand) ServerDetails() (*config.ServerDetails, error) {
	return uc.serverDetails, nil
}

func (uc *UpdateCommand) CommandName() string {
	return "skills_update"
}

func (uc *UpdateCommand) Run() error {
	installBase, err := uc.resolveInstallBase()
	if err != nil {
		return err
	}

	skillDir := filepath.Join(installBase, uc.slug)

	// Skill must already be installed
	currentVersion := uc.readInstalledVersion(skillDir)
	if currentVersion == "" {
		return fmt.Errorf("skill '%s' is not installed at %s\n\nTo install it first, run:\n  jf skills install %s --path %s",
			uc.slug, skillDir, uc.slug, installBase)
	}

	// Fetch available versions from Artifactory
	versions, err := common.ListVersions(uc.serverDetails, uc.repoKey, uc.slug)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return fmt.Errorf("skill '%s' not found in repository '%s'", uc.slug, uc.repoKey)
		}
		return fmt.Errorf("failed to fetch versions for '%s': %w", uc.slug, err)
	}
	if len(versions) == 0 {
		return fmt.Errorf("no versions found for skill '%s' in repository '%s'", uc.slug, uc.repoKey)
	}

	versionStrs := make([]string, len(versions))
	for i, v := range versions {
		versionStrs[i] = v.Version
	}

	// Resolve target version
	targetVersion, err := uc.resolveTargetVersion(versionStrs)
	if err != nil {
		return err
	}

	// Already on the right version
	if currentVersion == targetVersion && !uc.force {
		log.Info(fmt.Sprintf("Skill '%s' is already at version '%s'. Use --force to re-download.", uc.slug, currentVersion))
		return nil
	}

	if uc.force && currentVersion == targetVersion {
		log.Info(fmt.Sprintf("Re-downloading skill '%s' v%s (--force)", uc.slug, targetVersion))
	} else {
		log.Info(fmt.Sprintf("Updating skill '%s': %s → %s", uc.slug, currentVersion, targetVersion))
	}

	if uc.dryRun {
		log.Info("[dry-run] No files were modified.")
		return nil
	}

	// Ask for confirmation in interactive mode
	if !uc.quiet && !common.IsNonInteractive() {
		if !coreutils.AskYesNo(fmt.Sprintf("Update '%s' to v%s?", uc.slug, targetVersion), true) {
			return fmt.Errorf("update aborted by user")
		}
	}

	// Delegate download + unzip + copy + evidence verification to InstallCommand.
	// InstallCommand installs into <installBase>/<slug>/.
	ic := install.NewInstallCommand().
		SetServerDetails(uc.serverDetails).
		SetRepoKey(uc.repoKey).
		SetSlug(uc.slug).
		SetVersion(targetVersion).
		SetInstallPath(installBase).
		SetQuiet(uc.quiet)

	if err := ic.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	log.Info(fmt.Sprintf("Skill '%s' updated to v%s at %s", uc.slug, targetVersion, skillDir))
	return nil
}

// resolveInstallBase returns the parent directory in which the skill folder lives.
func (uc *UpdateCommand) resolveInstallBase() (string, error) {
	return common.ResolveInstallPath(uc.agentName, uc.projectDir, uc.global)
}

func (uc *UpdateCommand) resolveTargetVersion(available []string) (string, error) {
	if uc.version == "" || uc.version == "latest" {
		return common.LatestVersion(available)
	}

	for _, v := range available {
		if v == uc.version {
			return v, nil
		}
	}

	// Version not found — offer interactive selection if possible
	if uc.quiet || common.IsNonInteractive() {
		return "", fmt.Errorf("version '%s' not found for skill '%s'.\nAvailable versions: %s",
			uc.version, uc.slug, strings.Join(available, ", "))
	}

	log.Warn(fmt.Sprintf("Version '%s' not found for skill '%s'.", uc.version, uc.slug))
	options := make([]prompt.Suggest, len(available))
	for i, v := range available {
		options[i] = prompt.Suggest{Text: v}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		"Select a version:",
		fmt.Sprintf("'%%s' is not an available version for skill '%s'.", uc.slug),
		options,
	)
	return selected, nil
}

// readInstalledVersion reads the version from SKILL.md frontmatter.
// Returns "" if not installed or version field is absent.
func (uc *UpdateCommand) readInstalledVersion(skillDir string) string {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	return parseFrontmatterVersion(string(data))
}

func parseFrontmatterVersion(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			continue
		}
		if kv := strings.SplitN(trimmed, ":", 2); len(kv) == 2 {
			if strings.TrimSpace(kv[0]) == "version" {
				return strings.Trim(strings.TrimSpace(kv[1]), `"'`)
			}
		}
	}
	return ""
}

// RunUpdate is the CLI action for `jf skills update`.
func RunUpdate(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills update <slug> --agent <agent> [--global | --project-dir <path>] [--cross-agent] [--version <version>] [--repo <repo>] [--dry-run] [--force]")
	}

	slug := c.GetArgumentAt(0)

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := common.IsQuiet(c)

	agentName := c.GetStringFlagValue("agent")
	crossAgent := c.GetBoolFlagValue("cross-agent")
	global := c.GetBoolFlagValue("global")

	// Validate flags before doing any network calls.
	if agentName != "" && crossAgent {
		return fmt.Errorf("--agent and --cross-agent are mutually exclusive")
	}
	if global && c.GetStringFlagValue("project-dir") != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive")
	}
	if global && agentName == "" && !crossAgent {
		return fmt.Errorf("--global requires --agent to be specified")
	}

	// Resolve --cross-agent into the reserved agent name so the rest of the
	// code has a single agentName to work with.
	if crossAgent {
		agentName = common.CrossAgentName
	} else if agentName == "" {
		return fmt.Errorf("--agent is required. Supported agents: %s\n(Use --cross-agent to install into the shared .agents/skills directory.)",
			common.SupportedAgentsList())
	}

	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	projectDir := c.GetStringFlagValue("project-dir")
	if projectDir != "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("invalid --project-dir %q: %w", projectDir, err)
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return fmt.Errorf("--project-dir does not exist: %s", abs)
		}
		projectDir = abs
	}

	cmd := NewUpdateCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(c.GetStringFlagValue("version")).
		SetAgentName(agentName).
		SetProjectDir(projectDir).
		SetGlobal(global).
		SetDryRun(c.GetBoolFlagValue("dry-run")).
		SetForce(c.GetBoolFlagValue("force")).
		SetQuiet(quiet)

	return cmd.Run()
}
