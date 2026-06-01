package common

import (
	"path/filepath"
	"testing"

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
