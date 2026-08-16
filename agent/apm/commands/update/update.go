package update

import (
	"fmt"
	"os"
	"path/filepath"

	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// apmSubcommand is the apm subcommand this package always drives.
const apmSubcommand = "update"

// ApmUpdateCommand runs `apm update` with JFrog Artifactory authentication and collects
// build-info from the resulting apm.lock.yaml, reusing install's exact reader. Never accepts
// --repo; a registry must already be declared via jf setup apm or apm.yml's registries:
// block.
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

func (c *ApmUpdateCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApmUpdateCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *ApmUpdateCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *ApmUpdateCommand {
	c.buildConfiguration = buildConfiguration
	return c
}

func (c *ApmUpdateCommand) CommandName() string {
	return apmcommon.CommandNamePrefix + apmSubcommand
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

	if err := apmcommon.RunApmSubcommandWithAuth(apmSubcommand, c.args, c.serverDetails); err != nil {
		return fmt.Errorf("run apm update: %w", err)
	}

	// Only mention / collect build-info when the user asked for it (--build-name/--build-number or env).
	collectBuildInfo, err := apmcommon.ShouldCollectBuildInfo(c.buildConfiguration)
	if err != nil {
		log.Warn("apm update completed, but could not determine build-info collection state:", err.Error())
	} else if collectBuildInfo {
		if apmcommon.IsDryRunArg(c.args) {
			log.Info("apm update: --dry-run - nothing was updated, skipping build-info recording.")
		} else if workingDir, wdErr := os.Getwd(); wdErr != nil {
			log.Warn("apm update completed, but could not determine working directory for build info:", wdErr.Error())
		} else {
			lockfilePath := filepath.Join(workingDir, apmcommon.ApmLockfileName)
			manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
			if biErr := apmcommon.CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath, c.serverDetails, c.buildConfiguration); biErr != nil {
				log.Warn("apm update completed, but build info collection failed:", biErr.Error())
			}
		}
	}

	log.Info("apm update finished successfully.")
	return nil
}

// RunUpdate is the CLI action handler for `jf agent apm update`.
func RunUpdate(c *components.Context) error {
	if apmcommon.IsHelpRequest(c.Arguments) {
		return apmcommon.RunApmCommand(nil, apmSubcommand, []string{apmcommon.HelpFlag})
	}

	opts, err := apmcommon.ExtractApmSubcommandOptions(c.Arguments)
	if err != nil {
		return err
	}
	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	cmd := NewApmUpdateCommand().
		SetArgs(opts.RemainingArgs).
		SetServerDetails(serverDetails).
		SetBuildConfiguration(opts.BuildConfig)

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
