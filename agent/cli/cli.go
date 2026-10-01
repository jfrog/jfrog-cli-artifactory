package cli

import (
	apmcli "github.com/jfrog/jfrog-cli-artifactory/agent/apm/cli"
	pluginscli "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/cli"
	skillscli "github.com/jfrog/jfrog-cli-artifactory/agent/skills/cli"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetCommands returns the command groups under the `jf agent` namespace.
// Shared helpers live in agent/common.
func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "plugins",
			Description: "Agent plugin commands.",
			Subcommands: pluginscli.GetSubCommands(),
		},
		{
			Name:        "skills",
			Description: "Agent skill commands.",
			Subcommands: skillscli.GetSubCommands(),
		},
		{
			Name:          "apm",
			Description:   apmcli.GetDescription(),
			AIDescription: apmcli.GetAIDescription(),
			Subcommands:   apmcli.GetSubCommands(),
			Action:        apmcli.RunApmPassthroughDefault,
		},
	}
}
