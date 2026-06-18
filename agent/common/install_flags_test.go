package common

import (
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePathInstallBase_OK(t *testing.T) {
	abs, err := ResolvePathInstallBase(InstallFlagInput{PathInstallBase: t.TempDir()})
	require.NoError(t, err)
	assert.NotEmpty(t, abs)
}

func TestResolvePathInstallBase_NotPathMode(t *testing.T) {
	abs, err := ResolvePathInstallBase(InstallFlagInput{RawHarness: "cursor"})
	require.NoError(t, err)
	assert.Empty(t, abs)
}

func TestResolveInstallProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	abs, err := ResolveInstallProjectDir(projectDir, false)
	require.NoError(t, err)
	want, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, want, abs)
}

// validateInstallFlagsFixture is a minimal builtIns map for ValidateInstallFlags tests,
// mirroring the agents supported by the plugins package.
var validateInstallFlagsFixture = map[string]AgentConfig{
	"claude":      {ProjectDir: ".claude/plugins"},
	"cursor":      {ProjectDir: ".cursor/plugins"},
	"claude-code": {ProjectDir: ".claude/plugins"},
}

var validateInstallFlagsRegistryHelp = AgentRegistryHelpExample{
	ConfigSectionKey: PluginsAgentsKey,
}

func TestValidateInstallFlags_PathMode(t *testing.T) {
	testutil.WithJfrogHome(t)
	validPath := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("path", validPath)

	flags, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
	require.NoError(t, err)
	wantAbs, err := filepath.Abs(validPath)
	require.NoError(t, err)
	assert.True(t, flags.PathMode())
	assert.Equal(t, wantAbs, flags.AbsoluteInstallBaseDir)
	assert.Empty(t, flags.Specs)
	assert.Empty(t, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_HarnessProjectMode(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "claude")
	c.AddStringFlag("project-dir", projectDir)

	flags, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
	require.NoError(t, err)
	assert.False(t, flags.PathMode())
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "claude", flags.Specs[0].Name)
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_HarnessGlobalMode(t *testing.T) {
	testutil.WithJfrogHome(t)
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "cursor")
	c.AddBoolFlag("global", true)

	flags, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "cursor", flags.Specs[0].Name)
	assert.Empty(t, flags.ProjectDirAbs)
	assert.True(t, flags.IsGlobal)
}

func TestValidateInstallFlags_CommaSeparatedHarnesses(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "claude,cursor")
	c.AddStringFlag("project-dir", projectDir)

	flags, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 2)
	assert.Equal(t, "claude", flags.Specs[0].Name)
	assert.Equal(t, "cursor", flags.Specs[1].Name)
}

func TestValidateInstallFlags_UnknownAgent(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "my-agent")
	c.AddStringFlag("project-dir", projectDir)

	_, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestValidateInstallFlags_ConflictingFlags(t *testing.T) {
	testutil.WithJfrogHome(t)
	validPath := t.TempDir()
	projectDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func(*components.Context)
		wantSub string
	}{
		{
			name: "path with harness",
			setup: func(c *components.Context) {
				c.AddStringFlag("path", validPath)
				c.AddStringFlag("harness", "claude")
			},
			wantSub: "--path cannot be combined with --harness",
		},
		{
			name: "path with global",
			setup: func(c *components.Context) {
				c.AddStringFlag("path", validPath)
				c.AddBoolFlag("global", true)
			},
			wantSub: "--path cannot be combined with --global",
		},
		{
			name: "path with project-dir",
			setup: func(c *components.Context) {
				c.AddStringFlag("path", validPath)
				c.AddStringFlag("project-dir", projectDir)
			},
			wantSub: "--path cannot be combined with --project-dir",
		},
		{
			name:    "missing harness without path",
			setup:   func(*components.Context) {},
			wantSub: "--harness is required",
		},
		{
			name: "global and project-dir together",
			setup: func(c *components.Context) {
				c.AddStringFlag("harness", "claude")
				c.AddBoolFlag("global", true)
				c.AddStringFlag("project-dir", projectDir)
			},
			wantSub: "mutually exclusive",
		},
		{
			name: "path not a directory",
			setup: func(c *components.Context) {
				missing := filepath.Join(t.TempDir(), "nope")
				c.AddStringFlag("path", missing)
			},
			wantSub: "--path:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testutil.NewCLIContext()
			tt.setup(c)
			_, err := ValidateInstallFlags(c, validateInstallFlagsFixture, PluginsAgentsKey, validateInstallFlagsRegistryHelp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}
