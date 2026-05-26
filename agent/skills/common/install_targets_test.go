package common

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAgentTargets_PathMode(t *testing.T) {
	base := t.TempDir()
	targets, err := ResolveAgentTargets("my-skill", base, nil, "", false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(base, "my-skill"), targets[0].DestinationDir)
	assert.Equal(t, PathAgentName, targets[0].Agent.Name)
	assert.Equal(t, ScopePath, targets[0].Scope)
}

func TestValidateExistingDir_InvalidPath(t *testing.T) {
	err := ValidateExistingDir("/nonexistent/path/that/does/not/exist")
	require.Error(t, err)
}

func newInstallTestContext() *components.Context {
	ctx := &components.Context{}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
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
				c.AddStringFlag("path", validPath)
				c.AddStringFlag("harness", "cursor")
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
				c.AddStringFlag("harness", "cursor")
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
			c := newInstallTestContext()
			tt.setup(c)
			_, _, _, _, err := ValidateInstallFlags(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestValidateInstallFlags_PathModeOK(t *testing.T) {
	validPath := t.TempDir()
	c := newInstallTestContext()
	c.AddStringFlag("path", validPath)

	absPath, specs, projectDirAbs, isGlobal, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	wantAbs, err := filepath.Abs(validPath)
	require.NoError(t, err)
	assert.Equal(t, wantAbs, absPath)
	assert.Empty(t, specs)
	assert.Empty(t, projectDirAbs)
	assert.False(t, isGlobal)
}

func TestValidateInstallFlags_HarnessProjectOK(t *testing.T) {
	projectDir := t.TempDir()
	c := newInstallTestContext()
	c.AddStringFlag("harness", "cursor")
	c.AddStringFlag("project-dir", projectDir)

	absPath, specs, projectDirAbs, isGlobal, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	assert.Empty(t, absPath)
	require.Len(t, specs, 1)
	assert.Equal(t, "cursor", strings.ToLower(specs[0].Name))
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, projectDirAbs)
	assert.False(t, isGlobal)
}
