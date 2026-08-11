package cargo

import (
	"fmt"
	"os"
	"strings"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// applyEnv sets each "KEY=VALUE" entry into this process's environment and returns a restore
// closure that undoes every mutation (previous value on updates, unset on new keys). The
// build-info collector runs `cargo metadata` in its own subprocess (build-info-go) which
// inherits our environment, so the auth env injected for `cargo build` must also be applied
// here for metadata to resolve through the authenticated Artifactory registry — but the
// bearer token must not linger in this jf process's environment after collection completes,
// where it could leak to any later child or to an env dump. Callers MUST defer the returned
// closure right after the call:
//
//	restore := applyEnv(extraEnv)
//	defer restore()
func applyEnv(env []string) (restore func()) {
	var restores []func()
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		key := kv[:i]
		val := kv[i+1:]
		prev, had := os.LookupEnv(key)
		if err := os.Setenv(key, val); err != nil {
			log.Debug("cargo: could not set env " + key + ": " + err.Error())
			continue
		}
		if had {
			k, p := key, prev
			restores = append(restores, func() {
				if err := os.Setenv(k, p); err != nil {
					log.Debug("cargo: could not restore env " + k + ": " + err.Error())
				}
			})
		} else {
			k := key
			restores = append(restores, func() {
				if err := os.Unsetenv(k); err != nil {
					log.Debug("cargo: could not unset env " + k + ": " + err.Error())
				}
			})
		}
	}
	return func() {
		// Restore in reverse order so overlapping keys settle back to their original value.
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
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
	// auth so it can resolve through the authenticated Artifactory registry. The bearer token
	// stays in-process only for the duration of collection: the deferred restore undoes every
	// change so nothing later in this jf run inherits the auth env.
	restore := applyEnv(extraEnv)
	defer restore()

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
	// Never log the raw args: `cargo login <token>` and `cargo owner --token <token>` (also
	// `--token=<token>`) pass secrets on the command line. redactCargoArgs replaces those with
	// a placeholder so a debug-verbose run does not print bearer tokens verbatim.
	log.Debug(fmt.Sprintf("cargo: running '%s %v'", cargoExe, redactCargoArgs(args)))
	return runCmd(cfg)
}

// redactCargoArgs returns a copy of args with cargo secret-bearing arguments replaced by
// "<redacted>". Redacts:
//   - the positional token in `cargo login <TOKEN> [--registry <name>]`
//   - the value of a `--token <VAL>` flag pair
//   - the value inside `--token=<VAL>`
//
// The original slice is left untouched so cargo still receives the real token.
func redactCargoArgs(args []string) []string {
	const placeholder = "<redacted>"
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		switch {
		case a == "--token":
			if i+1 < len(out) {
				out[i+1] = placeholder
			}
		case strings.HasPrefix(a, "--token="):
			out[i] = "--token=" + placeholder
		}
	}
	// `cargo login <token>`: the token is the first non-flag positional after "login".
	if len(out) > 0 && out[0] == "login" {
		for i := 1; i < len(out); i++ {
			if !strings.HasPrefix(out[i], "-") {
				out[i] = placeholder
				break
			}
		}
	}
	return out
}
