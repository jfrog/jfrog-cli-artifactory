package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetSubCommands returns leaf commands for `jf agent plugins` (publish, install, …).
func GetSubCommands() []components.Command {
	return []components.Command{
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsPublish),
			Description: "Publish an agent plugin to Artifactory. Signs and attaches evidence if a signing key is provided.",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
		{
			Name:        "install",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsInstall),
			Description: "Install an agent plugin from Artifactory. Use --agent (comma-separated) with --project-dir or --global, or --path <dir> for a direct install to <dir>/<slug>. Agent paths come from ~/.jfrog/agents/agent-plugin-config.json with built-in fallbacks. Verifies evidence when signing keys are configured.",
			Arguments:   getInstallArguments(),
			Action:      install.RunInstall,
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the plugin folder containing plugin.json.",
		},
	}
}

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Plugin slug to install (the plugin's name in Artifactory).",
		},
	}
}
