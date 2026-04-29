package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserUVConfigPath(t *testing.T) {
	t.Run("respects UV_CONFIG_FILE env var", func(t *testing.T) {
		customPath := "/custom/path/to/uv.toml"
		t.Setenv("UV_CONFIG_FILE", customPath)
		path, err := getUserUVConfigPath()
		require.NoError(t, err)
		assert.Equal(t, customPath, path)
	})

	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("UV_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
		path, err := getUserUVConfigPath()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/custom/xdg", "uv", "uv.toml"), path)
	})

	t.Run("falls back to ~/.config/uv/uv.toml", func(t *testing.T) {
		t.Setenv("UV_CONFIG_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		path, err := getUserUVConfigPath()
		require.NoError(t, err)
		home, _ := os.UserHomeDir()
		assert.Equal(t, filepath.Join(home, ".config", "uv", "uv.toml"), path)
	})
}

func TestLoadUVConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")

	fullCfg, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	assert.Empty(t, fullCfg)
	assert.Nil(t, indexes)
}

func TestLoadUVConfig_WithIndexes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")

	content := `
[pip]
compile-bytecode = true

[[index]]
name = "fly-pypi"
url = "https://example.com/api/pypi/simple"
default = true

[[index]]
name = "other"
url = "https://other.example.com/simple"
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	fullCfg, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	assert.Contains(t, fullCfg, "pip")
	assert.Len(t, indexes, 2)
	assert.Equal(t, "fly-pypi", indexes[0].Name)
	assert.Equal(t, "https://example.com/api/pypi/simple", indexes[0].URL)
	assert.True(t, indexes[0].Default)
	assert.Equal(t, "other", indexes[1].Name)
}

func TestConfigureUVIndex_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config", "uv", "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	err := ConfigureUVIndex("https://example.com/api/pypi/simple")
	require.NoError(t, err)

	_, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	assert.Equal(t, uvIndexName, indexes[0].Name)
	assert.Equal(t, "https://example.com/api/pypi/simple", indexes[0].URL)
	assert.True(t, indexes[0].Default)
}

func TestConfigureUVIndex_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	content := `[[index]]
name = "fly-pypi"
url = "https://old.example.com/simple"
default = true
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	err := ConfigureUVIndex("https://new.example.com/simple")
	require.NoError(t, err)

	_, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	assert.Equal(t, "https://new.example.com/simple", indexes[0].URL)
}

func TestConfigureUVIndex_PreservesOtherSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	content := `[pip]
compile-bytecode = true

[[index]]
name = "other-index"
url = "https://other.example.com/simple"
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	err := ConfigureUVIndex("https://fly.example.com/simple")
	require.NoError(t, err)

	fullCfg, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	assert.Contains(t, fullCfg, "pip")
	require.Len(t, indexes, 2)

	var otherFound, flyFound bool
	for _, idx := range indexes {
		if idx.Name == "other-index" {
			otherFound = true
			assert.Equal(t, "https://other.example.com/simple", idx.URL)
		}
		if idx.Name == uvIndexName {
			flyFound = true
			assert.Equal(t, "https://fly.example.com/simple", idx.URL)
			assert.True(t, idx.Default)
		}
	}
	assert.True(t, otherFound, "other-index should be preserved")
	assert.True(t, flyFound, "fly-pypi index should be added")
}

func TestRemoveUVIndex(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	content := `[[index]]
name = "fly-pypi"
url = "https://fly.example.com/simple"
default = true

[[index]]
name = "other-index"
url = "https://other.example.com/simple"
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	err := RemoveUVIndex()
	require.NoError(t, err)

	_, indexes, err := loadUVConfig(configPath)
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	assert.Equal(t, "other-index", indexes[0].Name)
}

func TestRemoveUVIndex_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent", "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	err := RemoveUVIndex()
	require.NoError(t, err)
}

func TestGetConfiguredUVIndexURL(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")
	t.Setenv("UV_CONFIG_FILE", configPath)

	t.Run("returns URL when configured", func(t *testing.T) {
		content := `[[index]]
name = "fly-pypi"
url = "https://fly.example.com/simple"
default = true
`
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

		url, err := GetConfiguredUVIndexURL()
		require.NoError(t, err)
		assert.Equal(t, "https://fly.example.com/simple", url)
	})

	t.Run("returns empty when not configured", func(t *testing.T) {
		content := `[[index]]
name = "other-index"
url = "https://other.example.com/simple"
`
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

		url, err := GetConfiguredUVIndexURL()
		require.NoError(t, err)
		assert.Empty(t, url)
	})

	t.Run("returns empty when file missing", func(t *testing.T) {
		t.Setenv("UV_CONFIG_FILE", filepath.Join(tmpDir, "missing.toml"))

		url, err := GetConfiguredUVIndexURL()
		require.NoError(t, err)
		assert.Empty(t, url)
	})
}

func TestWriteUVConfig_RemovesIndexKeyWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "uv.toml")

	fullCfg := map[string]any{
		"pip": map[string]any{"compile-bytecode": true},
	}

	err := writeUVConfig(configPath, fullCfg, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "index")
	assert.Contains(t, string(data), "compile-bytecode")
}
