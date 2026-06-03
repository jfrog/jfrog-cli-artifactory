package list

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand_NoMode(t *testing.T) {
	cmd := &ListCommand{}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jf agent plugins list requires")
}

func TestListCommand_BothModes(t *testing.T) {
	cmd := &ListCommand{repoKey: "my-repo", agentNames: []string{"claude"}}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestListCommand_GlobalAndProjectDir(t *testing.T) {
	cmd := &ListCommand{agentNames: []string{"claude"}, global: true, projectDir: "/some/path"}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestBuildRowForPlugin_ManifestOnlyMatchesUpdate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "web")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".jfrog"), 0o755))
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, plugincommon.PluginInfoManifestFile, plugincommon.PluginInfoManifest{
		Repo:             "plugins-local",
		Slug:             "web",
		InstalledVersion: "2.0.0",
		Scope:            "global",
		Agent:            "claude",
	}))

	row, ok := (&ListCommand{}).buildRowForPlugin(dir, "web", "")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", row.Version)
	assert.Equal(t, "plugins-local", row.Repo)
	assert.Empty(t, row.Description)
}

func TestBuildRowForPlugin_SkipsWhenNotInstalled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	_, ok := (&ListCommand{}).buildRowForPlugin(dir, "cache", "")
	assert.False(t, ok)
}

func TestListCommand_CheckUpdatesWithRepo(t *testing.T) {
	cmd := &ListCommand{repoKey: "my-repo", checkUpdates: true}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--check-updates")
}
