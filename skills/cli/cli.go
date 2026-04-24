package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/search"
	skillsupdate "github.com/jfrog/jfrog-cli-artifactory/skills/commands/update"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetCommands() []components.Command {
	return []components.Command{
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
			Description: "Install a skill from Artifactory. Verifies evidence using Artifactory keys automatically.",
			Arguments:   getInstallArguments(),
			Action:      install.RunInstall,
		},
		{
			Name:        "search",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsSearch),
			Description: "Search for skills across Artifactory repositories.",
			Arguments:   getSearchArguments(),
			Action:      search.RunSearch,
		},
		{
			Name:        "update",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsUpdate),
			Description: "Update an installed skill to the latest (or a specific) version. Use --dry-run to preview and --force to re-download even when already up to date.",
			Arguments:   getUpdateArguments(),
			Action:      skillsupdate.RunUpdate,
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
			Description: "Skill name/slug to update (e.g. web-search).",
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
