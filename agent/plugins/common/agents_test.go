package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentRegistry_BuiltInDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)

	registry, err := LoadAgentRegistry()
	require.NoError(t, err)

	cursor, ok := registry["cursor"]
	require.True(t, ok)
	assert.False(t, cursor.FromConfig)
	assert.Equal(t, ".cursor/plugins", cursor.Config.ProjectDir)
	assert.Equal(t, "~/.cursor/plugins", cursor.Config.GlobalDir)
}

func TestLoadAgentRegistry_HonoursPluginConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv(coreutils.HomeDir, home)
	configPath := filepath.Join(home, "agents", PluginConfigFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"agents": {
			"my-agent": {"globalDir": "~/.my/plugins", "projectDir": ".my/plugins"}
		}
	}`), 0o644))

	registry, err := LoadAgentRegistry()
	require.NoError(t, err)
	custom, ok := registry["my-agent"]
	require.True(t, ok)
	assert.True(t, custom.FromConfig)
	assert.Equal(t, ".my/plugins", custom.Config.ProjectDir)
}

func TestSupportedAgentsList_ContainsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)
	got := SupportedAgentsList()
	assert.True(t, strings.Contains(got, "cursor"))
	assert.True(t, strings.Contains(got, "claude-code"))
}
