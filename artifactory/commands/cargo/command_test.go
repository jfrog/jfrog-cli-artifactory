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

func TestCargoInvocationArgs(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		args        []string
		want        []string
	}{
		{"subcommand with flags", "build", []string{"--all-features"}, []string{"build", "--all-features"}},
		{"subcommand no flags", "build", nil, []string{"build"}},
		{"publish with registry", "publish", []string{"--registry", "jfrog"}, []string{"publish", "--registry", "jfrog"}},
		{"no subcommand (bare flags)", "", []string{"--version"}, []string{"--version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cargoInvocationArgs(tt.commandName, tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("cargoInvocationArgs(%q, %v) = %v, want %v", tt.commandName, tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q (full: %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

