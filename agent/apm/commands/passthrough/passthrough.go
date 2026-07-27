package passthrough

import (
	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ApmPassthroughCommand forwards any apm subcommand with auth environment injected.
type ApmPassthroughCommand struct {
	subcmd        string
	args          []string
	serverDetails *config.ServerDetails
	repoName      string
}

func NewApmPassthroughCommand() *ApmPassthroughCommand {
	return &ApmPassthroughCommand{}
}

func (c *ApmPassthroughCommand) SetSubcmd(subcmd string) *ApmPassthroughCommand {
	c.subcmd = subcmd
	return c
}

func (c *ApmPassthroughCommand) SetArgs(args []string) *ApmPassthroughCommand {
	c.args = args
	return c
}

func (c *ApmPassthroughCommand) SetServerDetails(sd *config.ServerDetails) *ApmPassthroughCommand {
	c.serverDetails = sd
	return c
}

func (c *ApmPassthroughCommand) SetRepoName(repo string) *ApmPassthroughCommand {
	c.repoName = repo
	return c
}

func (c *ApmPassthroughCommand) CommandName() string {
	return "rt_agent_apm_" + c.subcmd
}

func (c *ApmPassthroughCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *ApmPassthroughCommand) Run() error {
	log.Info("Running apm " + c.subcmd + "...")
	return apmcommon.RunApmSubcommandWithAuth(c.subcmd, c.args, c.serverDetails, c.repoName)
}

// RunApmPassthroughDefault handles any `jf agent apm <subcmd>` where <subcmd> is not
// one of the registered subcommands (install, publish). The subcmd is extracted
// from the first element of c.Arguments; all remaining elements are forwarded to apm.
//
// The parent "apm" command can't declare SkipFlagParsing (jfrog-cli-core rejects that
// combined with registered Subcommands, since urfave/cli would then stop routing to
// install/publish entirely), so the framework's automatic flag parsing only reliably
// captures --server-id/--repo when they're placed BEFORE the subcommand name. Placed
// after — the position install/publish/every other jf command actually uses — they land
// here unconsumed. Extract them manually, position-independent, the same way every other
// passthrough-style command in the CLI (npm, pnpm, yarn, ...) does via
// coreutils.ExtractServerIdFromCommand, before forwarding the rest to apm.
func RunApmPassthroughDefault(c *components.Context) error {
	if len(c.Arguments) == 0 {
		return apmcommon.RunApmCommand(nil, "--help", nil)
	}

	subcmd := c.Arguments[0]
	if apmcommon.IsHelpRequest([]string{subcmd}) {
		return apmcommon.RunApmCommand(nil, "--help", nil)
	}

	rest, serverID, err := coreutils.ExtractServerIdFromCommand(c.Arguments[1:])
	if err != nil {
		return err
	}
	rest, repoOverride, err := coreutils.ExtractStringOptionFromArgs(rest, "repo")
	if err != nil {
		return err
	}

	sd, sdErr := agentcommon.GetServerDetails(c)
	if sdErr != nil || serverID != "" {
		// Either the framework-based lookup found nothing configured (flags were placed after
		// the subcommand, so agentcommon.GetServerDetails saw none of them), or an explicit
		// --server-id turned up in the manual scan above — resolve from that instead.
		sd, err = config.GetSpecificConfig(serverID, true, true)
		if err != nil {
			return err
		}
	}

	repoName := c.GetStringFlagValue("repo")
	if repoOverride != "" {
		repoName = repoOverride
	}

	cmd := NewApmPassthroughCommand().
		SetSubcmd(subcmd).
		SetArgs(rest).
		SetServerDetails(sd).
		SetRepoName(repoName)

	return commands.ExecWithPackageManager(cmd, "agent-apm")
}
