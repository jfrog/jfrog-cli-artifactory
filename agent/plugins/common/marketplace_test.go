package common

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarketplace_ClaudeFixture(t *testing.T) {
	marketplace, err := ParseMarketplace(filepath.Join("testdata", "claude-marketplace.json"))
	require.NoError(t, err)
	require.NotNil(t, marketplace)
	assert.Equal(t, "test-marketplace", marketplace.Name)
	require.Len(t, marketplace.Plugins, 2)
	assert.Equal(t, "dummy-plugin-alpha", marketplace.Plugins[0].Name)
	assert.Equal(t, "1.0.2", marketplace.Plugins[0].Version)
}

func TestParseMarketplace_CursorFixture(t *testing.T) {
	marketplace, err := ParseMarketplace(filepath.Join("testdata", "cursor-marketplace.json"))
	require.NoError(t, err)
	require.Len(t, marketplace.Plugins, 1)
	assert.Equal(t, "dummy-plugin-gamma", marketplace.Plugins[0].Name)
	assert.Equal(t, "1.0.1", marketplace.Plugins[0].Version)
}

func TestParseMarketplace_CodexFixtureWithExtraFields(t *testing.T) {
	marketplace, err := ParseMarketplace(filepath.Join("testdata", "codex-marketplace.json"))
	require.NoError(t, err)
	require.Len(t, marketplace.Plugins, 1)
	assert.Equal(t, "dummy-plugin-delta", marketplace.Plugins[0].Name)
	assert.Equal(t, "2.0.0-rc.1", marketplace.Plugins[0].Version)
}

func TestFindEntry(t *testing.T) {
	marketplace, err := ParseMarketplace(filepath.Join("testdata", "claude-marketplace.json"))
	require.NoError(t, err)

	entry, ok := FindEntry(marketplace, "dummy-plugin-beta")
	require.True(t, ok)
	assert.Equal(t, "1.0.1", entry.Version)

	_, ok = FindEntry(marketplace, "missing")
	assert.False(t, ok)

	// case-insensitive match
	entry, ok = FindEntry(marketplace, "Dummy-Plugin-Alpha")
	require.True(t, ok)
	assert.Equal(t, "1.0.2", entry.Version)
}

func TestParseMarketplace_MissingFile(t *testing.T) {
	_, err := ParseMarketplace(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestMarketplaceFileName(t *testing.T) {
	assert.Equal(t, "claude-marketplace.json", MarketplaceFileName("claude"))
	assert.Equal(t, "cursor-marketplace.json", MarketplaceFileName("Cursor"))
	assert.Equal(t, "my-agent-marketplace.json", MarketplaceFileName("  my-agent  "))
}
