package cli

import (
	pluginsCLI "github.com/jfrog/jfrog-cli-artifactory/ai/plugins/cli"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetAiCommands returns the command groups under the `jf ai` namespace.
// Shared helpers live in ai/common.
func GetAiCommands() []components.Command {
	return []components.Command{
		{
			Name:        "plugins",
			Description: "AI agent plugin commands.",
			Subcommands: pluginsCLI.GetSubCommands(),
		},
	}
}
