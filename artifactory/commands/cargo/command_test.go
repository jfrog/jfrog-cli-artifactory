package cargo

import "testing"

func TestCargoCommandImplementsInterface(t *testing.T) {
	c := NewCargoCommand().SetCommandName("build").SetArgs([]string{"build"})
	if c.CommandName() != "rt_cargo" {
		t.Errorf("CommandName = %q", c.CommandName())
	}
	if _, err := c.ServerDetails(); err != nil {
		t.Errorf("ServerDetails err: %v", err)
	}
	if c.commandName != "build" {
		t.Errorf("commandName not set: %q", c.commandName)
	}
}
