package cli

import (
	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/apm/commands/publish"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// GetSubCommands returns the leaf commands for `jf agent apm`. Commands not listed here (e.g.
// "lock", which resolves but doesn't collect build-info) fall through to the parent's
// passthrough handler.
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
	}
}

// RunApmPassthroughDefault handles any `jf agent apm <subcmd>` not among install/publish.
// Passthrough takes exactly one jf-level flag of its own, --server-id (falling back to the
// default configured JFrog server when absent); everything else in c.Arguments is forwarded to
// apm untouched.
func RunApmPassthroughDefault(c *components.Context) error {
	if len(c.Arguments) == 0 {
		return apmcommon.RunApmCommand(nil, apmcommon.HelpFlag, nil)
	}

	subcmd := c.Arguments[0]
	if apmcommon.IsHelpRequest([]string{subcmd}) {
		return apmcommon.RunApmCommand(nil, apmcommon.HelpFlag, nil)
	}

	// Strip --server-id before anything else touches the remaining args, so it never leaks
	// through to the native apm binary (which has no such option of its own) - the same
	// pattern jf nix's own passthrough fallback uses via coreutils.ExtractServerIdFromCommand.
	remainingArgs, serverID, err := coreutils.ExtractServerIdFromCommand(c.Arguments[1:])
	if err != nil {
		return err
	}

	// Show help without resolving server/auth, which a help request never needs. Forward the
	// full remaining arg tail so nested commands like "deps why" get their own help, not "deps"'s.
	if apmcommon.IsHelpRequest(remainingArgs) {
		return apmcommon.RunApmCommand(nil, subcmd, remainingArgs)
	}

	serverDetails, err := agentcommon.GetServerDetailsByID(serverID)
	if err != nil {
		return err
	}

	cmd := &apmcommon.PassthroughCommand{
		Subcmd: subcmd,
		Args:   remainingArgs,
		Server: serverDetails,
	}

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
