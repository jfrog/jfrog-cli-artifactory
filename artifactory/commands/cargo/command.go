package cargo

import (
	"fmt"
	"os"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
	flexpack "github.com/jfrog/build-info-go/flexpack"
)

// authGate reports whether auth env should be injected for this run:
// only in native mode, and only for commands that talk to the registry.
func authGate(native bool, commandName string) bool {
	return native && needsRemoteAccess(commandName)
}

// collectionBucket returns the build-info collection bucket for this run,
// or "none" when native mode is off (no collection in legacy pass-through).
func collectionBucket(native bool, commandName string) string {
	if !native {
		return "none"
	}
	return commandBucket(commandName)
}

// CargoCommand satisfies commands.Command and drives a native cargo invocation
// with optional JFrog Artifactory authentication and build-info collection.
type CargoCommand struct {
	commandName        string
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
}

// NewCargoCommand returns an empty CargoCommand ready for chained setters.
func NewCargoCommand() *CargoCommand { return &CargoCommand{} }

// SetCommandName sets the cargo sub-command (e.g. "build", "publish").
func (c *CargoCommand) SetCommandName(name string) *CargoCommand { c.commandName = name; return c }

// SetArgs sets the full argument list forwarded to cargo.
func (c *CargoCommand) SetArgs(args []string) *CargoCommand { c.args = args; return c }

// SetServerDetails stores the Artifactory server configuration.
func (c *CargoCommand) SetServerDetails(d *config.ServerDetails) *CargoCommand {
	c.serverDetails = d
	return c
}

// SetBuildConfiguration stores the build-info configuration.
func (c *CargoCommand) SetBuildConfiguration(b *buildUtils.BuildConfiguration) *CargoCommand {
	c.buildConfiguration = b
	return c
}

// CommandName returns the telemetry name used by commands.Exec.
func (c *CargoCommand) CommandName() string { return "rt_cargo" }

// ServerDetails returns the stored server configuration (satisfies commands.Command).
func (c *CargoCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run executes the full cargo workflow: resolve working dir, inject auth env,
// run native cargo, then dispatch build-info collection by command bucket.
func (c *CargoCommand) Run() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cargo: get working directory: %w", err)
	}
	c.workingDir = wd

	native := flexpack.IsFlexPackEnabled()

	var extraEnv []string
	if authGate(native, c.commandName) {
		extraEnv = c.resolveAuthEnv()
	}

	if err := c.runNativeCargo(extraEnv); err != nil {
		return err
	}

	switch collectionBucket(native, c.commandName) {
	case "deps":
		return c.collectDeps()
	case "artifacts":
		return c.collectArtifacts(false)
	case "publish":
		return c.collectArtifacts(true)
	default:
		return nil
	}
}

// runNativeCargo builds a CargoRunConfig and delegates to the exec.go wrapper.
func (c *CargoCommand) runNativeCargo(extraEnv []string) error {
	cargoExe := "cargo"
	cfg := &CargoRunConfig{Exe: cargoExe, Args: c.args, Dir: c.workingDir, ExtraEnv: extraEnv}
	log.Debug(fmt.Sprintf("cargo: running '%s %v'", cargoExe, c.args))
	return runCmd(cfg)
}
