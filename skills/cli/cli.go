package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	skillslist "github.com/jfrog/jfrog-cli-artifactory/skills/commands/list"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/search"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/update"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "list",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsList),
			Description: "List skills from an Artifactory repository (--repo) or from a local agent install (--agent). Exactly one of --repo or --agent is required. With --agent, use --project-dir for the project root (default: current directory) or --global for each agent's global directory.",
			Action:      skillslist.RunList,
			Hidden:      true,
		},
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsPublish),
			Description: "Publish a skill to Artifactory. Signs and attaches evidence if a signing key is provided. Runs Xray security scan after upload (use --skip-scan or JFROG_CLI_SKIP_SKILLS_SCAN=true to bypass). Scan timeout is configurable via JFROG_CLI_SKILLS_SCAN_TIMEOUT (default: 5m, e.g. 2m, 30s).",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
		{
			Name:        "install",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsInstall),
			Description: "Install a skill from Artifactory. Use --agent (comma-separated names) with --project-dir (default: current directory) or --global, or use --path <dir> for a direct install to <dir>/<slug> (same layout as skills update). Agent paths use ~/.jfrog/agents/agent-config.json with built-in fallbacks. Verifies evidence when signing keys are configured. Use --format json for machine-readable install summary.",
			Arguments:   getInstallArguments(),
			Action:      install.RunInstall,
		},
		{
			Name:        "update",
			Hidden:      true,
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsUpdate),
			Description: "Update an installed skill to the latest (or a specific) version. Same targeting flags as install: use --agent (comma-separated) with --project-dir (default: current directory) or --global, or --path <dir> for a direct update at <dir>/<slug>. Preflight skips targets that are not installed or already at the target version (use --force to re-download). Logs skip and failure reasons when not quiet. Downloads once for all targets. Use --dry-run to preview, --format json for machine-readable summaries.",
			Arguments:   getUpdateArguments(),
			Action:      update.RunUpdate,
		},
		{
			Name:        "search",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsSearch),
			Description: "Search for skills across Artifactory repositories.",
			Arguments:   getSearchArguments(),
			Action:      search.RunSearch,
		},
		{
			Name:        "delete",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsDelete),
			Description: "Delete a specific skill version from Artifactory.",
			Arguments:   getDeleteArguments(),
			Action:      delete.RunDelete,
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the skill folder containing SKILL.md.",
		},
	}
}

func getSearchArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "query",
			Description: "Skill name or search term.",
		},
	}
}

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to install.",
		},
	}
}

func getUpdateArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to update.",
		},
	}
}

func getDeleteArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to delete.",
		},
	}
}
