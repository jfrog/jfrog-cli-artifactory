package common

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAgentTargets_PathMode(t *testing.T) {
	base := t.TempDir()
	targets, err := ResolveAgentTargets("my-plugin", base, nil, "", false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	target := targets[0]
	assert.Equal(t, filepath.Join(base, "my-plugin"), target.DestinationDir)
	assert.Equal(t, PathAgentName, target.Agent.Name)
	assert.Equal(t, ScopePath, target.Scope)
}

func TestResolveAgentTargets_Project(t *testing.T) {
	projectRoot := t.TempDir()
	spec := AgentSpec{Name: "claude", Config: AgentConfig{ProjectDir: ".claude/plugins"}}
	targets, err := ResolveAgentTargets("my-plugin", "", []AgentSpec{spec}, projectRoot, false)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	target := targets[0]
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "plugins", "my-plugin"), target.DestinationDir)
	assert.Equal(t, ScopeProject, target.Scope)
}
