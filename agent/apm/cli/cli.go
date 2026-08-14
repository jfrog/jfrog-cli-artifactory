package cli

import (
	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/update"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
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

// RunApmPassthroughDefault handles any `jf agent apm <subcmd>` not among install/publish/update.
// Auth always comes from the default configured JFrog server; passthrough takes no flags of its
// own, so nothing is extracted from c.Arguments beyond the subcommand.
func RunApmPassthroughDefault(c *components.Context) error {
	if len(c.Arguments) == 0 {
		return apmcommon.RunApmCommand(nil, apmcommon.HelpFlag, nil)
	}

	subcmd := c.Arguments[0]
	if apmcommon.IsHelpRequest([]string{subcmd}) {
		return apmcommon.RunApmCommand(nil, apmcommon.HelpFlag, nil)
	}
	// Show help without resolving server/auth, which a help request never needs. Forward the
	// full remaining arg tail so nested commands like "deps why" get their own help, not "deps"'s.
	if apmcommon.IsHelpRequest(c.Arguments[1:]) {
		return apmcommon.RunApmCommand(nil, subcmd, c.Arguments[1:])
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	cmd := &apmcommon.PassthroughCommand{
		Subcmd: subcmd,
		Args:   c.Arguments[1:],
		Server: serverDetails,
	}

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
