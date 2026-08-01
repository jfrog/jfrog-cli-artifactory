package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/update"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetSubCommands returns the leaf commands for `jf agent apm`. Commands not listed here (e.g.
// "lock", which resolves but deploys nothing) fall through to the parent's passthrough handler.
func GetSubCommands() []components.Command {
	return []components.Command{
		{
			Name:  "install",
			Flags: flagkit.GetCommandFlags(flagkit.AgentApm),
			// SkipFlagParsing so apm-native flags (e.g. --frozen) that aren't in jf's own
			// declared flag set above aren't rejected by urfave/cli before reaching apm.
			// RunInstall extracts jf's own flags manually via ExtractApmSubcommandOptions.
			SkipFlagParsing: true,
			Description:     "Install APM packages with JFrog Artifactory authentication.",
			AIDescription:   install.GetAIDescription(),
			Action:          install.RunInstall,
		},
		{
			Name:            "publish",
			Flags:           flagkit.GetCommandFlags(flagkit.AgentApm),
			SkipFlagParsing: true,
			Description:     "Publish an APM package to JFrog Artifactory.",
			AIDescription:   publish.GetAIDescription(),
			Action:          publish.RunPublish,
		},
		{
			Name:            "update",
			Flags:           flagkit.GetCommandFlags(flagkit.AgentApm),
			SkipFlagParsing: true,
			Description:     "Refresh APM dependencies to their latest matching refs, with build-info collection.",
			AIDescription:   update.GetAIDescription(),
			Action:          update.RunUpdate,
		},
	}
}
