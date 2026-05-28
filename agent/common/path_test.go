package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde relative path", in: "~/x/y", want: filepath.Join(home, "x/y")},
		{name: "absolute path", in: "/abs/path", want: "/abs/path"},
		{name: "tilde only unchanged", in: "~", want: "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandHome(tt.in))
		})
	}
}

func TestValidateExistingDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ValidateExistingDir(dir))

	require.Error(t, ValidateExistingDir(filepath.Join(dir, "missing")))
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
