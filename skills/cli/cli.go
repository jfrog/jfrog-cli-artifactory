package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/search"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsPublish),
			Description: "Publish a skill to Artifactory. Signs and attaches evidence if a signing key is provided.",
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

func getDeleteArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to delete.",
		},
	}
}
