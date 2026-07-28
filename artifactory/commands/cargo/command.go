package cargo

import (
	"fmt"
	"os"
	"strings"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// applyEnv sets each "KEY=VALUE" entry into this process's environment. The build-info collector
// runs `cargo metadata` in its own subprocess (build-info-go) which inherits our environment, so
// the auth env injected for `cargo build` must also be applied here for metadata to resolve
// through the authenticated Artifactory registry.
func applyEnv(env []string) {
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			if err := os.Setenv(kv[:i], kv[i+1:]); err != nil {
				log.Debug("cargo: could not set env " + kv[:i] + ": " + err.Error())
			}
		}
	}
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

	var extraEnv []string
	if needsRemoteAccess(c.commandName) {
		extraEnv = c.resolveAuthEnv()
	}

	if err := c.runNativeCargo(extraEnv); err != nil {
		return err
	}

	// The collector's `cargo metadata` subprocess inherits this process's env — apply the same
	// auth so it can resolve through the authenticated Artifactory registry.
	applyEnv(extraEnv)

	switch commandBucket(c.commandName) {
	case "deps":
		return c.collectDeps()
	case "publish":
		return c.collectArtifacts()
	default:
		return nil
	}
}

// cargoInvocationArgs assembles the full argument list for the cargo binary,
// prepending the sub-command name (when present) to the forwarded args. The CLI
// layer splits the sub-command out of the args (getCommandName), so it must be
// restored here or cargo would run with no sub-command.
func cargoInvocationArgs(commandName string, args []string) []string {
	if commandName == "" {
		return args
	}
	return append([]string{commandName}, args...)
}

// runNativeCargo builds a CargoRunConfig and delegates to the exec.go wrapper.
func (c *CargoCommand) runNativeCargo(extraEnv []string) error {
	cargoExe := "cargo"
	args := cargoInvocationArgs(c.commandName, c.args)
	cfg := &CargoRunConfig{Exe: cargoExe, Args: args, Dir: c.workingDir, ExtraEnv: extraEnv}
	log.Debug(fmt.Sprintf("cargo: running '%s %v'", cargoExe, args))
	return runCmd(cfg)
}
