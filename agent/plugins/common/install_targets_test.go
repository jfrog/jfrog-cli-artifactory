package common

import (
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInstallTestContext() *components.Context {
	ctx := &components.Context{}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

func TestValidateInstallFlags_PathMode(t *testing.T) {
	withJfrogHome(t)
	validPath := t.TempDir()
	c := newInstallTestContext()
	c.AddStringFlag("path", validPath)

	absPath, spec, projectDirAbs, isGlobal, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	wantAbs, err := filepath.Abs(validPath)
	require.NoError(t, err)
	assert.Equal(t, wantAbs, absPath)
	assert.Empty(t, spec.Name)
	assert.Empty(t, projectDirAbs)
	assert.False(t, isGlobal)
}

func TestValidateInstallFlags_HarnessProjectMode(t *testing.T) {
	withJfrogHome(t)
	projectDir := t.TempDir()
	c := newInstallTestContext()
	c.AddStringFlag("harness", "claude")
	c.AddStringFlag("project-dir", projectDir)

	absPath, spec, projectDirAbs, isGlobal, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	assert.Empty(t, absPath)
	assert.Equal(t, "claude", spec.Name)
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, projectDirAbs)
	assert.False(t, isGlobal)
}

func TestValidateInstallFlags_HarnessGlobalMode(t *testing.T) {
	withJfrogHome(t)
	c := newInstallTestContext()
	c.AddStringFlag("harness", "cursor")
	c.AddBoolFlag("global", true)

	_, spec, projectDirAbs, isGlobal, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	assert.Equal(t, "cursor", spec.Name)
	assert.Empty(t, projectDirAbs)
	assert.True(t, isGlobal)
}

func TestValidateInstallFlags_HarnessRejectsCommaList(t *testing.T) {
	withJfrogHome(t)
	c := newInstallTestContext()
	c.AddStringFlag("harness", "claude,cursor")

	_, _, _, _, err := ValidateInstallFlags(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single harness name")
}

func TestValidateInstallFlags_UnknownAgent(t *testing.T) {
	withJfrogHome(t)
	projectDir := t.TempDir()
	c := newInstallTestContext()
	c.AddStringFlag("harness", "my-agent")
	c.AddStringFlag("project-dir", projectDir)

	_, _, _, _, err := ValidateInstallFlags(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestValidateInstallFlags_ConflictingFlags(t *testing.T) {
	withJfrogHome(t)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newInstallTestContext()
			tt.setup(c)
			_, _, _, _, err := ValidateInstallFlags(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestResolveAgentTarget_PathMode(t *testing.T) {
	base := t.TempDir()
	target, err := ResolveAgentTarget("my-plugin", base, AgentSpec{}, "", false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "my-plugin"), target.DestinationDir)
	assert.Equal(t, PathAgentName, target.Agent.Name)
	assert.Equal(t, ScopePath, target.Scope)
}

func TestResolveAgentTarget_Project(t *testing.T) {
	projectRoot := t.TempDir()
	spec := AgentSpec{Name: "claude", Config: AgentConfig{ProjectDir: ".claude/plugins"}}
	target, err := ResolveAgentTarget("my-plugin", "", spec, projectRoot, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "plugins", "my-plugin"), target.DestinationDir)
	assert.Equal(t, ScopeProject, target.Scope)
}
