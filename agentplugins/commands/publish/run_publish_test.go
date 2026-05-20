package publish

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPublishTestContext(args ...string) *components.Context {
	ctx := &components.Context{Arguments: args}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

func TestRunPublish_MissingPathArgument(t *testing.T) {
	err := RunPublish(newPublishTestContext())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage: jf ai plugins publish")
}

func TestValidatePluginDir(t *testing.T) {
	t.Run("path is not a directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "not-a-dir")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

		_, err := validatePluginDir(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid directory")
	})

	t.Run("path does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing-plugin-dir")
		_, err := validatePluginDir(missing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid directory")
	})

	t.Run("valid directory", func(t *testing.T) {
		pluginDir := t.TempDir()
		abs, err := validatePluginDir(pluginDir)
		require.NoError(t, err)
		info, err := os.Stat(abs)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}
