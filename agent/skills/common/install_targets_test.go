package common

import (
	"path/filepath"
	"testing"

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
