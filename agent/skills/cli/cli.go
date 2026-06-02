package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/install"
	skillslist "github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/list"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/search"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/update"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetSubCommands returns leaf commands for `jf agent skills`.
func GetSubCommands() []components.Command {
	return []components.Command{
		{
			Name:        "list",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsList),
			Description: "List skills in Artifactory or locally.",
			Action:      skillslist.RunList,
		},
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsPublish),
			Description: "Publish a skill to Artifactory.",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
		{
			Name:        "install",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsInstall),
			Description: "Install a skill from Artifactory.",
			Arguments:   getInstallArguments(),
			Action:      install.RunInstall,
		},
		{
			Name:        "update",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsUpdate),
			Description: "Update an installed skill.",
			Arguments:   getUpdateArguments(),
			Action:      update.RunUpdate,
		},
		{
			Name:        "search",
			Flags:       flagkit.GetCommandFlags(flagkit.SkillsSearch),
			Description: "Search for skills in Artifactory.",
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
			Description: "Skill slug to install.",
		},
	}
}

func getUpdateArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill slug to update.",
		},
	}
}

func getDeleteArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill slug to delete.",
		},
	}
}
