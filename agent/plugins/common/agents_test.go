package common

import (
	"path/filepath"
	"strings"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentRegistry_BuiltInsOnly(t *testing.T) {
	testutil.WithJfrogHome(t)

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
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{
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
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{
		"skills-agents": {"my-agent": {"projectDir": "x", "globalDir": "y"}}
	}`)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	_, ok := registry["my-agent"]
	assert.False(t, ok, "plugins registry must not include skills-agents entries")
}

func TestLoadAgentRegistry_RejectsEmptyEntry(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{"plugins-agents": {"broken": {}}}`)

	_, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must define globalDir and/or projectDir")
}

func TestParseHarnessList(t *testing.T) {
	got, err := ParseHarnessList("cursor, Claude")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor", "claude"}, got)
}

func TestParseHarnessList_EmptyAndDuplicates(t *testing.T) {
	_, err := ParseHarnessList("")
	require.Error(t, err)

	_, err = ParseHarnessList("cursor,,claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty name")

	_, err = ParseHarnessList("cursor,cursor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestResolveAgent_Unknown(t *testing.T) {
	testutil.WithJfrogHome(t)
	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	require.NoError(t, err)

	_, err = agentcommon.ResolveAgent(registry, "no-such-agent", RegistryHelp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Supported agents")
	assert.Contains(t, err.Error(), "claude")
}

func TestResolveAgentInstallDir_GlobalAndProject(t *testing.T) {
	testutil.WithJfrogHome(t)
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
	testutil.WithJfrogHome(t)
	got := agentcommon.SupportedAgentsList(Agents, agentcommon.PluginsAgentsKey)
	parts := strings.Split(got, ", ")
	assert.ElementsMatch(t, []string{"claude", "cursor", "codex"}, parts)
}
