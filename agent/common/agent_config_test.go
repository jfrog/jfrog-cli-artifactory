package common

import (
	"os"
	"path/filepath"
	"testing"

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

func TestLoadAgentConfigSection_MissingFile(t *testing.T) {
	withJfrogHome(t)

	section, path, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.NoError(t, err)
	assert.Nil(t, section)
	assert.NotEmpty(t, path)
}

func TestLoadAgentConfigSection_MissingKey(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{"plugins-agents": {"claude": {"projectDir": "x"}}}`)

	section, _, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.NoError(t, err)
	assert.Nil(t, section)
}

func TestLoadAgentConfigSection_ReturnsSection(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{
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
	home := withJfrogHome(t)
	writeAgentConfig(t, home, "not-json")

	_, _, err := LoadAgentConfigSection(SkillsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
