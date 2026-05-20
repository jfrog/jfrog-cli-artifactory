package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agentplugins/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetAiCommands returns the commands exposed under the `jf ai` namespace.
func GetAiCommands() []components.Command {
	return []components.Command{
		{
			Name:        "plugins",
			Description: "AI agent plugin commands.",
			Subcommands: []components.Command{getPublishCommand()},
		},
	}
}

func getPublishCommand() components.Command {
	return components.Command{
		Name:        "publish",
		Flags:       flagkit.GetCommandFlags(flagkit.AiPluginsPublish),
		Description: "Publish an AI agent plugin to Artifactory. Discovers plugin.json under the given directory (root and known agent subdirs), validates that all manifests agree on name and version, and uploads a zip to a repository of package type 'agentplugins'. Artifacts are stored in Artifactory at {repo}/{plugin-slug}/{version}/. Version precedence: --version flag, then consensus from manifests, then default 1.0.0. Upload succeeds even if evidence attachment fails; check logs for evidence warnings.",
		Arguments:   getPublishArguments(),
		Action:      publish.RunPublish,
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the plugin folder containing one or more plugin.json files.",
		},
	}
}
