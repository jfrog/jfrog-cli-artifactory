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
	return apmcommon.CommandNamePrefix + c.subcmd
}

func (c *ApmPassthroughCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *ApmPassthroughCommand) Run() error {
	log.Info("Running apm " + apmcommon.SanitizeLogValue(c.subcmd) + "...")
	return apmcommon.RunApmSubcommandWithAuth(c.subcmd, c.args, c.serverDetails)
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

	cmd := NewApmPassthroughCommand().
		SetSubcmd(subcmd).
		SetArgs(c.Arguments[1:]).
		SetServerDetails(serverDetails)

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
