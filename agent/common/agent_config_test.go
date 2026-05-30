package common

import (
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigPathForDisplay_ResolvedHome(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	got := AgentConfigPathForDisplay()
	want := filepath.Join(home, "agents", "agent-config.json")
	assert.Equal(t, want, got)
}

func TestLoadAgentConfigSection_MissingFile(t *testing.T) {
	testutil.WithJfrogHome(t)

	section, path, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.NoError(t, err)
	assert.Nil(t, section)
	assert.NotEmpty(t, path)
}

func TestLoadAgentConfigSection_MissingKey(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{"plugins-agents": {"claude": {"projectDir": "x"}}}`)

	section, _, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.NoError(t, err)
	assert.Nil(t, section)
}

func TestLoadAgentConfigSection_ReturnsSection(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{
		"skills-agents": {"cursor": {"projectDir": ".cursor/skills"}},
		"plugins-agents": {"claude": {"projectDir": ".claude/plugins"}},
		"plugin-manifest-paths": ["a", "b"]
	}`)

	section, _, err := LoadAgentConfigSection(PluginManifestPathsKey)
	require.NoError(t, err)
	require.NotNil(t, section)
	assert.Contains(t, string(section), `"a"`)
}

func TestLoadAgentConfigSection_BadJSON(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, "not-json")

	_, _, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
