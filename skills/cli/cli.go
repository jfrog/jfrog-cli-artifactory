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
			Name:  "publish",
			Flags: flagkit.GetCommandFlags(flagkit.SkillsPublish),
			Description: "Publish a skill to Artifactory.\n" +
				"  After uploading, evidence is signed and attached to the artifact.\n" +
				"  Provide a PGP private key via --signing-key (or EVD_SIGNING_KEY_PATH env var)\n" +
				"  and an alias via --key-alias (or EVD_KEY_ALIAS env var).\n" +
				"  If no key is provided, the upload succeeds but evidence creation is skipped.",
			Arguments: getPublishArguments(),
			Action:    publish.RunPublish,
		},
		{
			Name:  "install",
			Flags: flagkit.GetCommandFlags(flagkit.SkillsInstall),
			Description: "Install a skill from Artifactory.\n" +
				"  Evidence verification uses --use-artifactory-keys to pull the publisher's\n" +
				"  public key from Artifactory automatically. No local signing keys are needed.\n" +
				"  If verification fails, an interactive prompt lets you proceed or abort;\n" +
				"  in CI/quiet mode the install fails automatically.",
			Arguments: getInstallArguments(),
			Action:    install.RunInstall,
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the skill folder containing SKILL.md.",
		},
		{
			Name:        "repo",
			Description: "Skills repository key in Artifactory.",
		},
	}
}

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Skill name/slug to install.",
		},
		{
			Name:        "repo",
			Description: "Skills repository key in Artifactory.",
		},
	}
}
