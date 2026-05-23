package cli

import (
	pluginsCLI "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/cli"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetCommands returns the command groups under the `jf agent` namespace.
// Shared helpers live in agent/common.
func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "plugins",
			Description: "Agent plugin commands.",
			Subcommands: pluginsCLI.GetSubCommands(),
		},
	}
}
