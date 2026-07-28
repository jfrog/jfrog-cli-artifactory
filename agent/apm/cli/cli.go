package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/update"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetSubCommands returns the leaf commands for `jf agent apm`.
// Commands not listed here fall through to the passthrough handler set on the parent.
// "lock" is deliberately not listed here — it doesn't deploy anything, so there's nothing
// a build actually consumed to report; it's served by the generic passthrough like every
// other read/resolve-only apm command.
func GetSubCommands() []components.Command {
	return []components.Command{
		{
			Name:  "install",
			Flags: flagkit.GetCommandFlags(flagkit.AgentApmSubcommand),
			// SkipFlagParsing so apm-native flags (e.g. --frozen) that aren't in jf's own
			// declared flag set above aren't rejected by urfave/cli before reaching apm.
			// RunInstall extracts jf's own flags manually via ExtractApmSubcommandOptions.
			SkipFlagParsing: true,
			Description:     "Install APM packages with JFrog Artifactory authentication.",
			Action:          install.RunInstall,
		},
		{
			Name:            "publish",
			Flags:           flagkit.GetCommandFlags(flagkit.AgentApmSubcommand),
			SkipFlagParsing: true,
			Description:     "Publish an APM package to JFrog Artifactory.",
			Action:          publish.RunPublish,
		},
		{
			Name:            "update",
			Flags:           flagkit.GetCommandFlags(flagkit.AgentApmSubcommand),
			SkipFlagParsing: true,
			Description:     "Refresh APM dependencies to their latest matching refs, with build-info collection.",
			Action:          update.RunUpdate,
		},
	}
}
