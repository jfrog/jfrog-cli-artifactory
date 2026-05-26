package cli

import (
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
