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

func TestAuthGate(t *testing.T) {
	tests := []struct {
		name          string
		native        bool
		commandName   string
		expectGated   bool
	}{
		{
			name:        "native=false, build (remote access)",
			native:      false,
			commandName: "build",
			expectGated: false,
		},
		{
			name:        "native=true, build (remote access)",
			native:      true,
			commandName: "build",
			expectGated: true,
		},
		{
			name:        "native=true, metadata (no remote access)",
			native:      true,
			commandName: "metadata",
			expectGated: false,
		},
		{
			name:        "native=false, publish (remote access)",
			native:      false,
			commandName: "publish",
			expectGated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authGate(tt.native, tt.commandName)
			if result != tt.expectGated {
				t.Errorf("authGate(%v, %q) = %v, want %v", tt.native, tt.commandName, result, tt.expectGated)
			}
		})
	}
}

func TestCollectionBucket(t *testing.T) {
	tests := []struct {
		name          string
		native        bool
		commandName   string
		expectBucket  string
	}{
		{
			name:         "native=false, publish",
			native:       false,
			commandName:  "publish",
			expectBucket: "none",
		},
		{
			name:         "native=true, publish",
			native:       true,
			commandName:  "publish",
			expectBucket: "publish",
		},
		{
			name:         "native=true, build",
			native:       true,
			commandName:  "build",
			expectBucket: "deps",
		},
		{
			name:         "native=false, build",
			native:       false,
			commandName:  "build",
			expectBucket: "none",
		},
		{
			name:         "native=true, metadata",
			native:       true,
			commandName:  "metadata",
			expectBucket: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectionBucket(tt.native, tt.commandName)
			if result != tt.expectBucket {
				t.Errorf("collectionBucket(%v, %q) = %q, want %q", tt.native, tt.commandName, result, tt.expectBucket)
			}
		})
	}
}
