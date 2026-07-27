package update

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

// ApmUpdateCommand runs `apm update` with JFrog Artifactory authentication and collects
// build-info from the resulting apm.lock.yaml, reusing install's exact reader.
//
// Unlike passthrough commands, update never accepts --repo: no other package-manager
// integration in this CLI supports declaring a new repository at run time either - they all
// require the one-time `jf setup <tool>` step first. A registry must already be declared
// (via jf setup agent-apm or apm.yml's own registries: block) before update can authenticate
// against it.
type ApmUpdateCommand struct {
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
}

func NewApmUpdateCommand() *ApmUpdateCommand {
	return &ApmUpdateCommand{}
}

func (c *ApmUpdateCommand) SetArgs(args []string) *ApmUpdateCommand {
	c.args = args
	return c
}

func (c *ApmUpdateCommand) SetServerDetails(sd *config.ServerDetails) *ApmUpdateCommand {
	c.serverDetails = sd
	return c
}

func (c *ApmUpdateCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *ApmUpdateCommand {
	c.buildConfiguration = bc
	return c
}

func (c *ApmUpdateCommand) CommandName() string {
	return "rt_agent_apm_update"
}

func (c *ApmUpdateCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run wraps "apm update", which re-resolves dependencies to their latest matching refs and, on
// acceptance (interactive confirmation, or --yes for CI), rewrites both apm.yml and
// apm.lock.yaml. Build-info collection reuses install's exact reader — same resolved-dependency
// shape, whether the lockfile just changed or update reported nothing new.
func (c *ApmUpdateCommand) Run() error {
	log.Info("Running apm update...")

	if err := apmcommon.RunApmSubcommandWithAuth("update", c.args, c.serverDetails, ""); err != nil {
		return fmt.Errorf("run apm update: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Warn("apm update completed, but could not determine working directory for build info:", err.Error())
	} else {
		lockfilePath := filepath.Join(workingDir, apmcommon.ApmLockfileName)
		manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
		if biErr := apmcommon.CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath, c.serverDetails, c.buildConfiguration); biErr != nil {
			log.Warn("apm update completed, but build info collection failed:", biErr.Error())
		}
	}

	log.Info("apm update finished successfully.")
	return nil
}

// RunUpdate is the CLI action handler for `jf agent apm update`.
func RunUpdate(c *components.Context) error {
	if apmcommon.IsHelpRequest(c.Arguments) {
		return apmcommon.RunApmCommand(nil, "update", []string{"--help"})
	}

	opts, err := apmcommon.ExtractApmSubcommandOptions(c.Arguments)
	if err != nil {
		return err
	}

	cmd := NewApmUpdateCommand().
		SetArgs(opts.RemainingArgs).
		SetServerDetails(opts.ServerDetails).
		SetBuildConfiguration(opts.BuildConfig)

	return commands.ExecWithPackageManager(cmd, "agent-apm")
}
