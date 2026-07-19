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

	row, ok := (&ListCommand{}).buildRowForPlugin(dir, "web", "", "claude")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", row.Version)
	assert.Equal(t, "plugins-local", row.Repo)
	assert.Equal(t, emDash, row.Description, "no plugin.json on disk should show the same placeholder as other missing table values")
}

func TestBuildRowForPlugin_SkipsWhenNotInstalled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	_, ok := (&ListCommand{}).buildRowForPlugin(dir, "cache", "", "cursor")
	assert.False(t, ok)
}

func TestListCommand_CheckUpdatesWithRepo(t *testing.T) {
	cmd := &ListCommand{repoKey: "my-repo", checkUpdates: true}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--check-updates")
}

func TestListCommand_RejectsProjectScopeForClaude(t *testing.T) {
	restore := listNativePluginsFunc
	defer func() { listNativePluginsFunc = restore }()
	listNativePluginsFunc = func(string) ([]plugincommon.NativePluginInfo, error) {
		t.Fatal("must not list plugins when project scope is rejected up front")
		return nil, nil
	}

	// Explicit --project-dir (not --global): must be rejected, same as install/update.
	cmd := &ListCommand{agentNames: []string{"claude"}, projectDir: "/some/project"}
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude does not support project-scoped plugin lists")
	assert.Contains(t, err.Error(), "--global")
}

func TestListCommand_AllowsGlobalScopeForClaude(t *testing.T) {
	restore := listNativePluginsFunc
	defer func() { listNativePluginsFunc = restore }()
	listNativePluginsFunc = func(string) ([]plugincommon.NativePluginInfo, error) {
		return nil, nil
	}

	cmd := &ListCommand{agentNames: []string{"claude"}, global: true, format: "json"}
	require.NoError(t, cmd.Run())
}

func TestResolveListScope_DefaultsToGlobalWhenNothingGiven(t *testing.T) {
	assert.True(t, resolveListScope(false, "", true),
		"neither --global nor --project-dir given: must default to global, matching install/update's DefaultGlobalScope")
}

func TestResolveListScope_ExplicitProjectDirStaysProjectScope(t *testing.T) {
	assert.False(t, resolveListScope(false, "/some/project", true))
}

func TestResolveListScope_ExplicitGlobalStaysGlobal(t *testing.T) {
	assert.True(t, resolveListScope(true, "", true))
}

func TestResolveListScope_RepoModeUnaffected(t *testing.T) {
	assert.False(t, resolveListScope(false, "", false), "no --harness (repo mode): scope is irrelevant, must not be forced global")
}

func TestBuildPluginRowsForHarness_ClaudeUsesNativeListing(t *testing.T) {
	restore := listNativePluginsFunc
	defer func() { listNativePluginsFunc = restore }()
	listNativePluginsFunc = func(agentName string) ([]plugincommon.NativePluginInfo, error) {
		assert.Equal(t, "claude", agentName)
		return []plugincommon.NativePluginInfo{
			{Slug: "gopls-lsp", Repo: "claude-plugins-official", Version: "1.0.0", Path: "/home/u/.claude/plugins/cache/claude-plugins-official/gopls-lsp/1.0.0"},
			{Slug: "jfrog-plugin-test", Repo: "local", Version: "1.0.0", Path: "/home/u/.claude/plugins/cache/local/jfrog-plugin-test/1.0.0"},
		}, nil
	}

	rows, err := (&ListCommand{}).buildPluginRowsForHarness(nil, "claude")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// sorted by name ascending: gopls-lsp before jfrog-plugin-test
	assert.Equal(t, "gopls-lsp", rows[0].Name)
	assert.Equal(t, "claude-plugins-official", rows[0].Repo)
	assert.Equal(t, "1.0.0", rows[0].Version)
	assert.Equal(t, "jfrog-plugin-test", rows[1].Name)
	assert.Equal(t, "local", rows[1].Repo)
}

func TestBuildPluginRowsForHarness_CursorUsesDirScan(t *testing.T) {
	restore := listNativePluginsFunc
	defer func() { listNativePluginsFunc = restore }()
	listNativePluginsFunc = func(string) ([]plugincommon.NativePluginInfo, error) {
		t.Fatal("listNativePluginsFunc must not be called for agents without a native registry")
		return nil, nil
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "cursor-plugins", "web")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".jfrog"), 0o755))
	require.NoError(t, agentcommon.WriteInstallInfoManifest(pluginDir, plugincommon.PluginInfoManifestFile, plugincommon.PluginInfoManifest{
		Repo:             "plugins-local",
		Slug:             "web",
		InstalledVersion: "2.0.0",
		Scope:            "global",
		Agent:            "cursor",
	}))

	registry := map[string]agentcommon.AgentSpec{
		"cursor": {
			Config: agentcommon.AgentConfig{GlobalDir: filepath.Join(dir, "cursor-plugins")},
		},
	}

	rows, err := (&ListCommand{global: true}).buildPluginRowsForHarness(registry, "cursor")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "web", rows[0].Name)
	assert.Equal(t, "2.0.0", rows[0].Version)
}
