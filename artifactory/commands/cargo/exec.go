package cargo

import (
	"os"
	"os/exec"
)

// CargoRunConfig holds the parameters for a native cargo invocation.
type CargoRunConfig struct {
	Exe      string
	Args     []string
	Dir      string
	ExtraEnv []string
}

// runCmd executes the cargo command described by c, streaming stdout/stderr to
// the parent process and merging ExtraEnv on top of the current environment.
func runCmd(c *CargoRunConfig) error {
	cmd := exec.Command(c.Exe, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.ExtraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
