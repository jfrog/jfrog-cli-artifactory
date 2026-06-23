package common

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveAgentTargets_PathMode verifies that when an explicit base directory
// is provided (path mode), a single target is returned whose destination is
// base/packageName and whose scope is InstallScopePath.
func TestResolveAgentTargets_PathMode(t *testing.T) {
	base := t.TempDir()
	targets, err := ResolveAgentTargets("my-package", base, nil, "", false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(base, "my-package"), targets[0].DestinationDir)
	assert.Equal(t, PathAgentName, targets[0].Agent.Name)
	assert.Equal(t, InstallScopePath, targets[0].Scope)
}

// TestResolveAgentTargets_Project verifies that when an agent spec with a
// project-scoped directory is provided, the target destination is resolved
// relative to the project root and the scope is InstallScopeProject.
func TestResolveAgentTargets_Project(t *testing.T) {
	projectRoot := t.TempDir()
	spec := AgentSpec{Name: "claude", Config: AgentConfig{ProjectDir: ".claude/plugins"}}
	targets, err := ResolveAgentTargets("my-package", "", []AgentSpec{spec}, projectRoot, false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "plugins", "my-package"), targets[0].DestinationDir)
	assert.Equal(t, InstallScopeProject, targets[0].Scope)
}

func TestBuildPathInstallTarget(t *testing.T) {
	base := t.TempDir()
	target, err := BuildPathInstallTarget("my-package", base)
	require.NoError(t, err)
	assert.Equal(t, PathAgentName, target.Agent.Name)
	assert.Equal(t, InstallScopePath, target.Scope)
	wantDest, err := filepath.Abs(filepath.Join(base, "my-package"))
	require.NoError(t, err)
	assert.Equal(t, wantDest, target.DestinationDir)
}
