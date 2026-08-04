package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// apmSubcommand is the apm subcommand this package always drives.
const apmSubcommand = "install"

// ApmInstallCommand runs `apm install` with JFrog Artifactory authentication and collects
// build-info from the resulting apm.lock.yaml. Never accepts --repo; a registry must already be
// declared via jf setup agent-apm or apm.yml's own registries: block.
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

func (c *ApmInstallCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApmInstallCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *ApmInstallCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *ApmInstallCommand {
	c.buildConfiguration = buildConfiguration
	return c
}

func (c *ApmInstallCommand) CommandName() string {
	return apmcommon.CommandNamePrefix + apmSubcommand
}

func (c *ApmInstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *ApmInstallCommand) Run() error {
	log.Info("Running apm install...")

	if err := apmcommon.RunApmSubcommandWithAuth(apmSubcommand, c.args, c.serverDetails); err != nil {
		return fmt.Errorf("run apm install: %w", err)
	}

	// Only mention / collect build-info when the user asked for it (--build-name/--build-number or env).
	collectBuildInfo, err := apmcommon.ShouldCollectBuildInfo(c.buildConfiguration)
	if err != nil {
		log.Warn("apm install completed, but could not determine build-info collection state:", err.Error())
	} else if collectBuildInfo {
		if apmcommon.IsDryRunArg(c.args) {
			log.Info("apm install: --dry-run - nothing was installed, skipping build-info recording.")
		} else if apmcommon.IsGlobalArg(c.args) {
			log.Info("apm install: --global installs to ~/.apm, not the project directory - skipping build-info recording.")
		} else if workingDir, wdErr := os.Getwd(); wdErr != nil {
			log.Warn("apm install completed, but could not determine working directory for build info:", wdErr.Error())
		} else {
			lockfileDir := workingDir
			if rootDir := rootDirFromArgs(c.args); rootDir != "" {
				lockfileDir = rootDir
				if !filepath.IsAbs(lockfileDir) {
					lockfileDir = filepath.Join(workingDir, lockfileDir)
				}
			}
			lockfilePath := filepath.Join(lockfileDir, apmcommon.ApmLockfileName)
			manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
			if biErr := apmcommon.CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath, c.serverDetails, c.buildConfiguration); biErr != nil {
				log.Warn("apm install completed, but build info collection failed:", biErr.Error())
			}
		}
	}

	log.Info("apm install finished successfully.")
	return nil
}

// rootDirFromArgs extracts the value of --root, which redirects apm_modules/ and apm.lock.yaml
// under DIR instead of the working directory (apm.yml and .apm/ still resolve from $PWD).
// Returns "" if --root isn't present.
func rootDirFromArgs(args []string) string {
	for i, arg := range args {
		if arg == "--root" && i+1 < len(args) {
			return args[i+1]
		}
		if cut, ok := strings.CutPrefix(arg, "--root="); ok {
			return cut
		}
	}
	return ""
}

// RunInstall is the CLI action handler for `jf agent apm install`.
func RunInstall(c *components.Context) error {
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

	cmd := NewApmInstallCommand().
		SetArgs(opts.RemainingArgs).
		SetServerDetails(serverDetails).
		SetBuildConfiguration(opts.BuildConfig)

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
