package common

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadPluginInfoManifest(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "my-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, agentcommon.InstallDirMode))

	manifest := PluginInfoManifest{
		Repo:             "plugins-local",
		Slug:             "my-plugin",
		InstalledVersion: "1.0.3",
		Scope:            "project",
		Agent:            "claude",
		ProjectDir:       "/abs/project",
	}
	require.NoError(t, WritePluginInfoManifest(pluginDir, manifest))

	assert.Equal(t, "plugin-info.json", filepath.Base(PluginInfoManifestPath(pluginDir)))

	got, err := ReadPluginInfoManifest(pluginDir)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, PluginInfoManifestSchemaVersion, got.SchemaVersion)
	assert.Equal(t, "plugins-local", got.Repo)
	assert.Equal(t, "my-plugin", got.Slug)
	assert.Equal(t, "1.0.3", got.InstalledVersion)
	assert.Equal(t, "project", got.Scope)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "/abs/project", got.ProjectDir)
}

func TestReadPluginInfoManifest_MissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadPluginInfoManifest(dir)
	require.NoError(t, err)
	assert.Nil(t, got)
}
