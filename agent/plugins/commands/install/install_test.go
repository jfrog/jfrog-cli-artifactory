package install

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Prevent real agent CLI binaries from being invoked during unit tests, and make
	// results independent of whether claude/codex happen to be installed on the
	// machine running the tests (e.g. CI runners don't have them on PATH).
	plugincommon.ClaudeExec = func(_ ...string) error { return nil }
	plugincommon.CodexExec = func(_ ...string) error { return nil }
	plugincommon.LookPathClaude = func() (string, error) { return "/usr/bin/claude", nil }
	plugincommon.LookPathCodex = func() (string, error) { return "/usr/bin/codex", nil }
}

func TestResolveAgentTargetDirectories_ProjectScope(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := NewInstallCommand().
		SetSlug("my-plugin").
		SetAgents([]plugincommon.AgentSpec{{Name: "claude", Config: plugincommon.AgentConfig{ProjectDir: ".claude/plugins"}}}).
		SetProjectDir(projectRoot).
		SetGlobal(false) // Explicitly set to project scope

	// Claude does not support project scope - should error
	targets, err := cmd.resolveAgentTargetDirectories()
	require.Error(t, err)
	assert.Nil(t, targets)
	assert.Contains(t, err.Error(), "claude does not support project-scoped plugin installs")
}

func TestResolveAgentTargetDirectories_DefaultScopeUsesGlobalForCursor(t *testing.T) {
	globalBase := filepath.Join(t.TempDir(), "global", ".cursor", "plugins", "local")
	wantBase, err := filepath.Abs(globalBase)
	require.NoError(t, err)

	cmd := NewInstallCommand().
		SetSlug("jfrog-plugin-timepass").
		SetAgents([]plugincommon.AgentSpec{{Name: "cursor", Config: plugincommon.AgentConfig{GlobalDir: globalBase}}})

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(wantBase, "jfrog-plugin-timepass"), targets[0].DestinationDir)
	assert.Equal(t, plugincommon.ScopeGlobal, targets[0].Scope)
}

func TestResolveAgentTargetDirectories_GlobalScope(t *testing.T) {
	globalBase := filepath.Join(t.TempDir(), "global", ".cursor", "plugins")
	wantBase, err := filepath.Abs(globalBase)
	require.NoError(t, err)

	cmd := NewInstallCommand().
		SetSlug("alpha").
		SetAgents([]plugincommon.AgentSpec{{Name: "cursor", Config: plugincommon.AgentConfig{GlobalDir: globalBase}}}).
		SetGlobal(true)

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(wantBase, "alpha"), targets[0].DestinationDir)
}

func TestResolveAgentTargetDirectories_LegacyInstallPath(t *testing.T) {
	tmp := t.TempDir()
	cmd := NewInstallCommand().SetSlug("legacy").SetInstallPath(tmp)
	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(tmp, "legacy"), targets[0].DestinationDir)
}

func TestResolveVersion_ExplicitOverridesMarketplace(t *testing.T) {
	restore := resolvePluginVersion
	resolvePluginVersion = func(_ *config.ServerDetails, _, slug, requested string, quiet bool) (string, error) {
		assert.Equal(t, "my-plugin", slug)
		assert.Equal(t, "1.0.0", requested)
		assert.False(t, quiet)
		return "1.0.0", nil
	}
	t.Cleanup(func() { resolvePluginVersion = restore })

	cmd := NewInstallCommand().
		SetSlug("my-plugin").
		SetAgents([]plugincommon.AgentSpec{{Name: "claude"}}).
		SetVersion("1.0.0")

	got, err := cmd.resolveVersion()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", got)
}

func TestResolveVersion_EmptyVersionWithPathResolvesLatest(t *testing.T) {
	restore := resolvePluginVersion
	resolvePluginVersion = func(_ *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
		assert.Equal(t, "plugins-repo", repoKey)
		assert.Equal(t, "my-plugin", slug)
		assert.Equal(t, "", requested)
		assert.False(t, quiet)
		return "1.2.3", nil
	}
	t.Cleanup(func() { resolvePluginVersion = restore })

	cmd := NewInstallCommand().
		SetSlug("my-plugin").
		SetRepoKey("plugins-repo").
		SetInstallPath(t.TempDir())

	got, err := cmd.resolveVersion()
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", got)
}

func TestCopyExtractedToTargets_WritesPluginInfoManifest(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.json"), []byte(`{"name":"my-plugin","version":"1.0.0"}`), 0o644))
	dest := filepath.Join(t.TempDir(), "my-plugin")
	projectRoot := t.TempDir()

	ic := NewInstallCommand().
		SetRepoKey("plugins-repo").
		SetSlug("my-plugin").
		SetVersion("1.2.3").
		SetProjectDir(projectRoot)

	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		DestinationDir: dest,
		Scope:          plugincommon.ScopeProject,
	}
	rows := ic.CopyExtractedToTargets(src, []plugincommon.AgentTarget{target})
	require.Len(t, rows, 1)
	assert.Equal(t, agentcommon.SummaryStatusOK, rows[0].Status)

	got, err := agentcommon.ReadInstallInfoManifest(dest, plugincommon.PluginInfoManifestFile)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "plugins-repo", got.Repo)
	assert.Equal(t, "my-plugin", got.Slug)
	assert.Equal(t, "1.2.3", got.InstalledVersion)
	assert.Equal(t, "project", got.Scope)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, projectRoot, got.ProjectDir)
}

func TestRun_MissingHarnessAndPath(t *testing.T) {
	cmd := NewInstallCommand().SetSlug("my-plugin")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--harness is required")
}
