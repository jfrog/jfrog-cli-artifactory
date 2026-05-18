package cli

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/agentplugins/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var pluginPathArgument = components.Argument{
	Name:        "path",
	Description: "Path to the plugin folder containing one or more plugin.json files.",
}

// GetAiCommands returns the commands exposed under the `jf ai` namespace.
//
// The plugin component framework supports only one level of namespace nesting,
// so `jf ai plugins publish` is implemented as a single `plugins` command that
// dispatches on its first positional argument. The flag set is the union of all
// subcommand flags (today just publish).
func GetAiCommands() []components.Command {
	return []components.Command{
		{
			Name:        "plugins",
			Flags:       flagkit.GetCommandFlags(flagkit.AiPluginsPublish),
			Description: "AI agent plugin commands. Use 'jf ai plugins publish <path>' to publish a plugin to Artifactory.",
			Arguments:   getPluginsArguments(),
			Action:      runPluginsDispatcher,
		},
	}
}

// GetAgentPluginsCommands returns the commands exposed under the `jf ai-plugins`
// namespace (the framework-flat equivalent of `jf ai plugins`).
func GetAgentPluginsCommands() []components.Command {
	return []components.Command{
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.AiPluginsPublish),
			Description: "Publish an AI agent plugin to Artifactory. Discovers plugin.json under the given directory (root and known agent subdirs), validates that all manifests agree on name and version, and uploads a zip to a local repository of package type 'agentplugins'.",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{pluginPathArgument}
}

func getPluginsArguments() []components.Argument {
	return []components.Argument{
		{Name: "subcommand", Description: "Subcommand to run. Supported: 'publish'."},
		pluginPathArgument,
	}
}

// runPluginsDispatcher routes `jf ai plugins <sub> ...` to the right action.
// The framework provides a flat Command list, so we dispatch on the first arg.
func runPluginsDispatcher(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf ai plugins <subcommand> [args]. Supported subcommands: publish")
	}
	switch sub := c.GetArgumentAt(0); sub {
	case "publish":
		return publish.RunPublishFromDispatcher(c)
	default:
		return fmt.Errorf("unknown 'jf ai plugins' subcommand %q. Supported: publish", sub)
	}
}
