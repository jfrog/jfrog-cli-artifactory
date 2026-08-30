package cargo

import (
	"io"
	"os"
	"os/exec"

	gofrogcmd "github.com/jfrog/gofrog/io"
)

// CargoRunConfig holds the parameters for a native cargo invocation.
// It implements gofrogcmd's CmdConfig interface.
type CargoRunConfig struct {
	Exe      string
	Args     []string
	Dir      string
	ExtraEnv []string
}

// GetCmd builds the *exec.Cmd, keeping ExtraEnv scoped to the child process
// (merged on top of the current environment) rather than mutating the parent.
//
// Interactive cargo subcommands (login, owner add, yank with prompt, ...) read from stdin —
// cargo login in particular reads the API token from stdin when it is not passed as an
// argument. Wiring os.Stdin through to the child is what makes those work when running via
// `jf cargo …`; without it the child sees a closed stdin and either hangs or fails
// immediately, which was Naveen's "native cargo login command is not working" report.
func (c *CargoRunConfig) GetCmd() *exec.Cmd {
	cmd := exec.Command(c.Exe, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.ExtraEnv...)
	cmd.Stdin = os.Stdin
	return cmd
}

// GetEnv returns no process-level env vars; auth env is applied on the child in GetCmd.
func (c *CargoRunConfig) GetEnv() map[string]string { return map[string]string{} }

// GetStdWriter returns nil so gofrogcmd streams stdout to os.Stdout.
func (c *CargoRunConfig) GetStdWriter() io.WriteCloser { return nil }

// GetErrWriter returns nil so gofrogcmd streams stderr to os.Stderr.
func (c *CargoRunConfig) GetErrWriter() io.WriteCloser { return nil }

// runCmd executes the cargo command described by c via gofrogcmd (matches conan).
func runCmd(c *CargoRunConfig) error {
	return gofrogcmd.RunCmd(c)
}
