package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withJfrogHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)
	return dir
}

func writeAgentConfig(t *testing.T, home, body string) {
	t.Helper()
	path := filepath.Join(home, "agents", "agent-config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLoadAgentRegistry_BuiltInsOnly(t *testing.T) {
	withJfrogHome(t)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	for _, name := range []string{"claude", "cursor", "codex"} {
		spec, ok := registry[name]
		require.True(t, ok, "expected built-in %q", name)
		assert.False(t, spec.FromConfig)
	}
	// Built-in registry must be exactly the supported plugin agents.
	assert.Len(t, registry, 3)
}

func TestLoadAgentRegistry_OverridesAndAdds(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{
		"plugins-agents": {
			"cursor": {"globalDir": "/abs/cursor", "projectDir": ".override/cursor"},
			"my-agent": {"globalDir": "~/.my/plugins", "projectDir": ".my/plugins"}
		}
	}`)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	cursor := registry["cursor"]
	assert.True(t, cursor.FromConfig)
	assert.Equal(t, ".override/cursor", cursor.Config.ProjectDir)

	custom, ok := registry["my-agent"]
	require.True(t, ok)
	assert.True(t, custom.FromConfig)

	claude := registry["claude"]
	assert.False(t, claude.FromConfig)
	assert.Equal(t, ".claude/plugins", claude.Config.ProjectDir)
}

func TestLoadAgentRegistry_IgnoresSkillsAgents(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{
		"skills-agents": {"my-agent": {"projectDir": "x", "globalDir": "y"}}
	}`)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	_, ok := registry["my-agent"]
	assert.False(t, ok, "plugins registry must not include skills-agents entries")
}

func TestLoadAgentRegistry_RejectsEmptyEntry(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{"plugins-agents": {"broken": {}}}`)

	_, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must define globalDir and/or projectDir")
}

func TestParseSingleHarness(t *testing.T) {
	got, err := ParseSingleHarness("Claude")
	require.NoError(t, err)
	assert.Equal(t, "claude", got)

	_, err = ParseSingleHarness("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	_, err = ParseSingleHarness("claude,cursor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single harness name")
}

func TestResolveAgent_Unknown(t *testing.T) {
	withJfrogHome(t)
	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	_, err = agentcommon.ResolveAgent(registry, "no-such-agent", RegistryHelp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Supported agents")
	assert.Contains(t, err.Error(), "claude")
}

func TestResolveAgentInstallDir_GlobalAndProject(t *testing.T) {
	withJfrogHome(t)
	spec := AgentSpec{Name: "cursor", Config: AgentConfig{GlobalDir: "/abs/cursor/plugins", ProjectDir: ".cursor/plugins"}}

	abs, err := agentcommon.ResolveAgentInstallDir(spec, "", true)
	require.NoError(t, err)
	want, err := filepath.Abs("/abs/cursor/plugins")
	require.NoError(t, err)
	assert.Equal(t, want, abs)

	projectRoot := t.TempDir()
	abs, err = agentcommon.ResolveAgentInstallDir(spec, projectRoot, false)
	require.NoError(t, err)
	wantProject, err := filepath.Abs(filepath.Join(projectRoot, spec.Config.ProjectDir))
	require.NoError(t, err)
	assert.Equal(t, wantProject, abs)
}

func TestSupportedAgentsList_OnlyPluginAgents(t *testing.T) {
	withJfrogHome(t)
	got := agentcommon.SupportedAgentsList(Agents, agentcommon.PluginsAgentsKey)
	parts := strings.Split(got, ", ")
	assert.ElementsMatch(t, []string{"claude", "cursor", "codex"}, parts)
}
