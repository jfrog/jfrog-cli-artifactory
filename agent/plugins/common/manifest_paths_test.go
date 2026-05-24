package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveManifestSearchPaths_DefaultsOnly(t *testing.T) {
	t.Setenv(coreutils.HomeDir, t.TempDir())
	got := ResolveManifestSearchPaths()
	assert.Equal(t, KnownManifestRelPaths, got)
}

func TestResolveManifestSearchPaths_AppendsExtras(t *testing.T) {
	home := t.TempDir()
	t.Setenv(coreutils.HomeDir, home)
	configPath := filepath.Join(home, "agents", PluginConfigFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"manifestPaths": ["custom/plugin.json", ".cursor-plugin/plugin.json", "  spaced/plugin.json  "]
	}`), 0o644))

	got := ResolveManifestSearchPaths()
	require.Greater(t, len(got), len(KnownManifestRelPaths))
	for index, builtIn := range KnownManifestRelPaths {
		assert.Equal(t, builtIn, got[index])
	}
	assert.Contains(t, got, "custom/plugin.json")
	assert.Contains(t, got, "spaced/plugin.json")
	// Hardcoded ".cursor-plugin/plugin.json" must not appear twice.
	occurrences := 0
	for _, path := range got {
		if path == ".cursor-plugin/plugin.json" {
			occurrences++
		}
	}
	assert.Equal(t, 1, occurrences)
}

func TestResolveManifestSearchPaths_BadJSONFallsBackToDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv(coreutils.HomeDir, home)
	configPath := filepath.Join(home, "agents", PluginConfigFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte("not-json"), 0o644))

	got := ResolveManifestSearchPaths()
	assert.Equal(t, KnownManifestRelPaths, got)
}

func TestFindPrimaryPluginManifest_UsesConfigExtensions(t *testing.T) {
	home := t.TempDir()
	t.Setenv(coreutils.HomeDir, home)
	configPath := filepath.Join(home, "agents", PluginConfigFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{"manifestPaths": ["custom/plugin.json"]}`), 0o644))

	pluginRoot := t.TempDir()
	customDir := filepath.Join(pluginRoot, "custom")
	require.NoError(t, os.MkdirAll(customDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(customDir, "plugin.json"), []byte(`{"name":"x","version":"1.0.0"}`), 0o644))

	meta, err := ValidateAndResolvePluginMeta(pluginRoot, "")
	require.NoError(t, err)
	assert.Equal(t, "x", meta.Name)
	assert.Equal(t, "1.0.0", meta.Version)
}
