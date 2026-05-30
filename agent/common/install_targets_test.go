package common

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
