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

func TestResolveAgentTargets_PathMode(t *testing.T) {
	base := t.TempDir()
	targets, err := ResolveAgentTargets("my-skill", base, nil, "", false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(base, "my-skill"), targets[0].DestinationDir)
	assert.Equal(t, PathAgentName, targets[0].Agent.Name)
	assert.Equal(t, ScopePath, targets[0].Scope)
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
			c := testutil.NewCLIContext()
			tt.setup(c)
			_, err := ValidateInstallFlags(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestValidateInstallFlags_PathModeOK(t *testing.T) {
	validPath := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("path", validPath)

	flags, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	wantAbs, err := filepath.Abs(validPath)
	require.NoError(t, err)
	assert.True(t, flags.PathMode())
	assert.Equal(t, wantAbs, flags.AbsoluteInstallBaseDir)
	assert.Empty(t, flags.Specs)
	assert.Empty(t, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_HarnessProjectOK(t *testing.T) {
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "cursor")
	c.AddStringFlag("project-dir", projectDir)

	flags, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	assert.False(t, flags.PathMode())
	require.Len(t, flags.Specs, 1)
	assert.Equal(t, "cursor", strings.ToLower(flags.Specs[0].Name))
	wantProj, err := filepath.Abs(projectDir)
	require.NoError(t, err)
	assert.Equal(t, wantProj, flags.ProjectDirAbs)
	assert.False(t, flags.IsGlobal)
}

func TestValidateInstallFlags_CommaSeparatedHarnesses(t *testing.T) {
	projectDir := t.TempDir()
	c := testutil.NewCLIContext()
	c.AddStringFlag("harness", "cursor,claude-code")
	c.AddStringFlag("project-dir", projectDir)

	flags, err := ValidateInstallFlags(c)
	require.NoError(t, err)
	require.Len(t, flags.Specs, 2)
	assert.Equal(t, "cursor", flags.Specs[0].Name)
	assert.Equal(t, "claude-code", flags.Specs[1].Name)
}
