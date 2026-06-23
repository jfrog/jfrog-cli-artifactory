package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDockerBuildContextFromArgs(t *testing.T) {
	t.Run("last positional arg is context", func(t *testing.T) {
		args := []string{"build", "-t", "img:tag", "--push", "-f", "Dockerfile", "/tmp/mycontext"}
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, "/tmp/mycontext", ctx)
	})

	t.Run("defaults to dot when no positional context", func(t *testing.T) {
		wd, err := os.Getwd()
		require.NoError(t, err)
		args := []string{"build", "-t", "img:tag", "."}
		ctx, err := ExtractDockerBuildContextFromArgs(args)
		require.NoError(t, err)
		assert.Equal(t, wd, ctx)
	})
}
