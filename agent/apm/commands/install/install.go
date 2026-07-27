package install

import (
	"fmt"
	"os"
	"path/filepath"

	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ApmInstallCommand runs `apm install` with JFrog Artifactory authentication and collects
// build-info from the resulting apm.lock.yaml.
//
// Unlike passthrough commands, install never accepts --repo: no other package-manager
// integration in this CLI supports declaring a new repository at run time either - they all
// require the one-time `jf setup <tool>` step first (§3, "jf setup agent-apm"). A registry
// must already be declared (via jf setup agent-apm or apm.yml's own registries: block) before
// install can authenticate against it.
type ApmInstallCommand struct {
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
}

func NewApmInstallCommand() *ApmInstallCommand {
	return &ApmInstallCommand{}
}

func (c *ApmInstallCommand) SetArgs(args []string) *ApmInstallCommand {
	c.args = args
	return c
}

func (c *ApmInstallCommand) SetServerDetails(sd *config.ServerDetails) *ApmInstallCommand {
	c.serverDetails = sd
	return c
}

func (c *ApmInstallCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *ApmInstallCommand {
	c.buildConfiguration = bc
	return c
}

func (c *ApmInstallCommand) CommandName() string {
	return "rt_agent_apm_install"
}

func (c *ApmInstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *ApmInstallCommand) Run() error {
	log.Info("Running apm install...")

	if err := apmcommon.RunApmSubcommandWithAuth("install", c.args, c.serverDetails, ""); err != nil {
		return fmt.Errorf("run apm install: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Warn("apm install completed, but could not determine working directory for build info:", err.Error())
	} else {
		lockfilePath := filepath.Join(workingDir, apmcommon.ApmLockfileName)
		manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
		if biErr := apmcommon.CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath, c.serverDetails, c.buildConfiguration); biErr != nil {
			log.Warn("apm install completed, but build info collection failed:", biErr.Error())
		}
	}

	log.Info("apm install finished successfully.")
	return nil
}

// RunInstall is the CLI action handler for `jf agent apm install`.
func RunInstall(c *components.Context) error {
	if apmcommon.IsHelpRequest(c.Arguments) {
		return apmcommon.RunApmCommand(nil, "install", []string{"--help"})
	}

	opts, err := apmcommon.ExtractApmSubcommandOptions(c.Arguments)
	if err != nil {
		return err
	}

	cmd := NewApmInstallCommand().
		SetArgs(opts.RemainingArgs).
		SetServerDetails(opts.ServerDetails).
		SetBuildConfiguration(opts.BuildConfig)

	return commands.ExecWithPackageManager(cmd, "agent-apm")
}
