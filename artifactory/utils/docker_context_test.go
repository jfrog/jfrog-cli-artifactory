package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDockerBuildContextFromArgs(t *testing.T) {
	t.Run("last positional arg is context", func(t *testing.T) {
		args := []string{"build", "-t", "img:tag", "--push", "-f", "Dockerfile", "/tmp/mycontext"}
		want, err := filepath.Abs("/tmp/mycontext")
		require.NoError(t, err)
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, want, ctx)
	})

	t.Run("defaults to dot when no positional context", func(t *testing.T) {
		wd, err := os.Getwd()
		require.NoError(t, err)
		args := []string{"build", "-t", "img:tag", "."}
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, wd, ctx)
	})

	t.Run("context before trailing flags", func(t *testing.T) {
		contextDir := t.TempDir()
		args := []string{"build", contextDir, "-t", "img:tag"}
		want, err := filepath.Abs(contextDir)
		require.NoError(t, err)
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, want, ctx)
	})

	t.Run("context before interspersed flags", func(t *testing.T) {
		contextDir := t.TempDir()
		args := []string{"build", contextDir, "--build-arg", "FOO=bar", "-t", "org/service-a:1.0"}
		want, err := filepath.Abs(contextDir)
		require.NoError(t, err)
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, want, ctx)
	})
}

func TestResolveWorkingDirectoryFromDockerArgs(t *testing.T) {
	contextDir := t.TempDir()
	args := []string{"build", contextDir, "--build-arg", "FOO=bar", "-t", "org/service-a:1.0"}

	got, err := ResolveWorkingDirectoryFromDockerArgs(args)
	require.NoError(t, err)
	want, err := filepath.Abs(contextDir)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
