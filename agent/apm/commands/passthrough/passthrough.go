package passthrough

import (
	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ApmPassthroughCommand forwards any apm subcommand with auth environment injected.
type ApmPassthroughCommand struct {
	subcmd        string
	args          []string
	serverDetails *config.ServerDetails
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

func (c *ApmPassthroughCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApmPassthroughCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *ApmPassthroughCommand) CommandName() string {
	return "rt_agent_apm_" + c.subcmd
}

func (c *ApmPassthroughCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *ApmPassthroughCommand) Run() error {
	log.Info("Running apm " + apmcommon.SanitizeLogValue(c.subcmd) + "...")
	return apmcommon.RunApmSubcommandWithAuth(c.subcmd, c.args, c.serverDetails)
}

// RunApmPassthroughDefault handles any `jf agent apm <subcmd>` where <subcmd> is not one of the
// registered subcommands (install/publish/update). The subcmd is the first element of
// c.Arguments; every remaining element is forwarded to apm untouched. Auth always comes from
// the default configured JFrog server - passthrough takes no flags of its own at all, so there's
// nothing to extract from c.Arguments.
func RunApmPassthroughDefault(c *components.Context) error {
	if len(c.Arguments) == 0 {
		return apmcommon.RunApmCommand(nil, "--help", nil)
	}

	subcmd := c.Arguments[0]
	if apmcommon.IsHelpRequest([]string{subcmd}) {
		return apmcommon.RunApmCommand(nil, "--help", nil)
	}
	// e.g. "jf agent apm deps why --help" - show apm's own help for that (sub)subcommand rather
	// than falling through to ExecWithPackageManager, which would resolve server details and
	// inject auth env for a command that never actually needs them. Forward the full remaining
	// arg tail (not just a bare "--help") so nested commands like "deps why" keep their own
	// subcommand and apm shows help for the right level, not just "apm deps --help".
	if apmcommon.IsHelpRequest(c.Arguments[1:]) {
		return apmcommon.RunApmCommand(nil, subcmd, c.Arguments[1:])
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	cmd := NewApmPassthroughCommand().
		SetSubcmd(subcmd).
		SetArgs(c.Arguments[1:]).
		SetServerDetails(serverDetails)

	return commands.ExecWithPackageManager(cmd, "agent-apm")
}
