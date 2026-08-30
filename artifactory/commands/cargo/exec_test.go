package cargo

import (
	"testing"

	gofrogcmd "github.com/jfrog/gofrog/io"
)

// TestCargoRunConfigGetCmd verifies that GetCmd builds the command correctly
// with the specified working directory, arguments, and environment.
func TestCargoRunConfigGetCmd(t *testing.T) {
	c := &CargoRunConfig{
		Exe:      "cargo",
		Args:     []string{"build", "--all-features"},
		Dir:      "/tmp/x",
		ExtraEnv: []string{"FOO=bar"},
	}
	cmd := c.GetCmd()

	// Check working directory
	if cmd.Dir != "/tmp/x" {
		t.Errorf("expected cmd.Dir=/tmp/x, got %q", cmd.Dir)
	}

	// Check arguments
	expectedArgs := []string{"cargo", "build", "--all-features"}
	if len(cmd.Args) != len(expectedArgs) {
		t.Errorf("expected %d args, got %d", len(expectedArgs), len(cmd.Args))
	}
	for i, expected := range expectedArgs {
		if cmd.Args[i] != expected {
			t.Errorf("args[%d]: expected %q, got %q", i, expected, cmd.Args[i])
		}
	}

	// Check that ExtraEnv is present in cmd.Env
	hasExtra := false
	for _, envvar := range cmd.Env {
		if envvar == "FOO=bar" {
			hasExtra = true
			break
		}
	}
	if !hasExtra {
		t.Errorf("cmd.Env does not contain FOO=bar: %v", cmd.Env)
	}

	// Check that os.Environ vars are inherited (cmd.Env should be longer than 1)
	if len(cmd.Env) <= 1 {
		t.Errorf("cmd.Env should contain inherited os.Environ, but only has %d entries", len(cmd.Env))
	}
}

// TestCargoRunConfigWriters verifies that GetEnv, GetStdWriter, and GetErrWriter
// return the expected values.
func TestCargoRunConfigWriters(t *testing.T) {
	c := &CargoRunConfig{
		Exe:  "cargo",
		Args: []string{"build"},
		Dir:  "/tmp/x",
	}

	// Check GetEnv returns empty map
	env := c.GetEnv()
	if len(env) != 0 {
		t.Errorf("expected GetEnv to return empty map, got %v", env)
	}

	// Check GetStdWriter returns nil
	if c.GetStdWriter() != nil {
		t.Errorf("expected GetStdWriter to return nil, got %v", c.GetStdWriter())
	}

	// Check GetErrWriter returns nil
	if c.GetErrWriter() != nil {
		t.Errorf("expected GetErrWriter to return nil, got %v", c.GetErrWriter())
	}
}

// TestCargoRunConfigImplementsCmdConfig is a compile-time assertion that
// CargoRunConfig implements gofrogcmd.CmdConfig.
func TestCargoRunConfigImplementsCmdConfig(t *testing.T) {
	var _ gofrogcmd.CmdConfig = (*CargoRunConfig)(nil)
	// If this compiles, the assertion passes.
}
