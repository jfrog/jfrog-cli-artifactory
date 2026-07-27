package cli

import (
	apmcli "github.com/jfrog/jfrog-cli-artifactory/agent/apm/cli"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/passthrough"
	pluginscli "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/cli"
	skillscli "github.com/jfrog/jfrog-cli-artifactory/agent/skills/cli"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
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
			Name:        "apm",
			Description: "Agent Package Manager (APM) commands with JFrog Artifactory authentication.",
			Flags:       flagkit.GetCommandFlags(flagkit.ApmPassthrough),
			Subcommands: apmcli.GetSubCommands(),
			Action:      passthrough.RunApmPassthroughDefault,
		},
	}
}
