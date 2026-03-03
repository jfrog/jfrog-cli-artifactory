package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetCommands() []components.Command {
	return []components.Command{
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

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to install.",
		},
	}
}
