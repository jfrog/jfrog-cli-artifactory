package common

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSkillsAgents = map[string]AgentConfig{
	"claude-code": {GlobalDir: "~/.claude/skills", ProjectDir: ".claude/skills"},
	"cursor":      {GlobalDir: "~/.cursor/skills", ProjectDir: ".cursor/skills"},
}

var testPluginsAgents = map[string]AgentConfig{
	"claude": {GlobalDir: "~/.claude/plugins", ProjectDir: ".claude/plugins"},
	"cursor": {GlobalDir: "~/.cursor/plugins", ProjectDir: ".cursor/plugins"},
}

var testSkillsHelp = AgentRegistryHelpExample{
	ConfigSectionKey:  SkillsAgentsKey,
	ExampleProjectDir: ".my-agent/skills",
	ExampleGlobalDir:  "~/.my-agent/skills",
}

var testPluginsHelp = AgentRegistryHelpExample{
	ConfigSectionKey:  PluginsAgentsKey,
	ExampleProjectDir: ".my-agent/plugins",
	ExampleGlobalDir:  "~/.my-agent/plugins",
}

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

func TestValidateInstallFlags_Errors(t *testing.T) {
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
				c.AddStringFlag(InstallPathFlag, validPath)
				c.AddStringFlag(InstallHarnessFlag, "cursor")
			},
			wantSub: "--path cannot be combined with --harness",
		},
		{
			name: "path with global",
			setup: func(c *components.Context) {
				c.AddStringFlag(InstallPathFlag, validPath)
				c.AddBoolFlag(InstallGlobalFlag, true)
			},
			wantSub: "--path cannot be combined with --global",
		},
		{
			name: "path with project-dir",
			setup: func(c *components.Context) {
				c.AddStringFlag(InstallPathFlag, validPath)
				c.AddStringFlag(InstallProjectDirFlag, projectDir)
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
				c.AddStringFlag(InstallHarnessFlag, "cursor")
				c.AddBoolFlag(InstallGlobalFlag, true)
				c.AddStringFlag(InstallProjectDirFlag, projectDir)
			},
			wantSub: "mutually exclusive",
		},
		{
			name: "path not a directory",
			setup: func(c *components.Context) {
				missing := filepath.Join(t.TempDir(), "nope")
				c.AddStringFlag(InstallPathFlag, missing)
			},
			wantSub: "--path:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testutil.NewCLIContext()
			tt.setup(c)
			_, err := ValidateInstallFlags(c, testSkillsAgents, SkillsAgentsKey, testSkillsHelp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestValidateInstallFlags_PathModeOK(t *testing.T) {
	validPath := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallPathFlag, validPath)

	flags, err := ValidateInstallFlags(c, testSkillsAgents, SkillsAgentsKey, testSkillsHelp)
	require.NoError(t, err)
	wantAbs, err := filepath.Abs(validPath)
	require.NoError(t, err)
	assert.True(t, flags.PathMode())
	assert.Equal(t, wantAbs, flags.AbsoluteInstallBaseDir)
	assert.Empty(t, flags.Specs)
	assert.Empty(t, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_SkillsHarnessProjectOK(t *testing.T) {
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "cursor")
	c.AddStringFlag(InstallProjectDirFlag, projectDir)

	flags, err := ValidateInstallFlags(c, testSkillsAgents, SkillsAgentsKey, testSkillsHelp)
	require.NoError(t, err)
	assert.False(t, flags.PathMode())
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "cursor", strings.ToLower(flags.Specs[0].Name))
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_SkillsCommaSeparatedHarnesses(t *testing.T) {
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "cursor,claude-code")
	c.AddStringFlag(InstallProjectDirFlag, projectDir)

	flags, err := ValidateInstallFlags(c, testSkillsAgents, SkillsAgentsKey, testSkillsHelp)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 2)
	assert.Equal(t, "cursor", flags.Specs[0].Name)
	assert.Equal(t, "claude-code", flags.Specs[1].Name)
}

func TestValidateInstallFlags_PluginsHarnessProjectOK(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "claude")
	c.AddStringFlag(InstallProjectDirFlag, projectDir)

	flags, err := ValidateInstallFlags(c, testPluginsAgents, PluginsAgentsKey, testPluginsHelp)
	require.NoError(t, err)
	assert.False(t, flags.PathMode())
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "claude", flags.Specs[0].Name)
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_PluginsHarnessGlobalOK(t *testing.T) {
	testutil.WithJfrogHome(t)
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "cursor")
	c.AddBoolFlag(InstallGlobalFlag, true)

	flags, err := ValidateInstallFlags(c, testPluginsAgents, PluginsAgentsKey, testPluginsHelp)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "cursor", flags.Specs[0].Name)
	assert.Empty(t, flags.ProjectDirAbs)
	assert.True(t, flags.IsGlobal)
}

func TestValidateInstallFlags_PluginsCommaSeparatedHarnesses(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "claude,cursor")
	c.AddStringFlag(InstallProjectDirFlag, projectDir)

	flags, err := ValidateInstallFlags(c, testPluginsAgents, PluginsAgentsKey, testPluginsHelp)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 2)
	assert.Equal(t, "claude", flags.Specs[0].Name)
	assert.Equal(t, "cursor", flags.Specs[1].Name)
}

func TestValidateInstallFlags_UnknownAgent(t *testing.T) {
	testutil.WithJfrogHome(t)
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag(InstallHarnessFlag, "my-agent")
	c.AddStringFlag(InstallProjectDirFlag, projectDir)

	_, err := ValidateInstallFlags(c, testPluginsAgents, PluginsAgentsKey, testPluginsHelp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}
