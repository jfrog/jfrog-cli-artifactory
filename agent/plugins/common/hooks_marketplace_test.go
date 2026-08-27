package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Prevent real agent CLI binaries from being invoked during unit tests.
	ClaudeExec = func(_ ...string) error { return nil }
	CodexExec = func(_ ...string) error { return nil }
	LookPathClaude = func() (string, error) { return "", nil }
	LookPathCodex = func() (string, error) { return "", nil }
}

func TestUpsertLocalMarketplaceEntry_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "my-plugin", "1.0.0", "test-repo"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "my-plugin", m.Plugins[0].Name)
	assert.Equal(t, "1.0.0", m.Plugins[0].Version)
	assert.Equal(t, "./my-plugin", m.Plugins[0].Source)
}

func TestUpsertLocalMarketplaceEntry_SetsOwner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "my-plugin", "1.0.0", "test-repo"))

	m := readMarketplaceFile(t, path)
	assert.Equal(t, "JFrog", m.Owner.Name)
}

func TestUpsertLocalMarketplaceEntry_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "my-plugin", "1.0.0", "test-repo"))
	require.NoError(t, upsertLocalMarketplaceEntry(path, "my-plugin", "2.0.0", "test-repo"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 1, "upsert should replace, not append")
	assert.Equal(t, "2.0.0", m.Plugins[0].Version)
}

func TestUpsertLocalMarketplaceEntry_CaseInsensitiveMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "My-Plugin", "1.0.0", "test-repo"))
	require.NoError(t, upsertLocalMarketplaceEntry(path, "my-plugin", "2.0.0", "test-repo"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 1, "case-insensitive upsert should replace")
	assert.Equal(t, "2.0.0", m.Plugins[0].Version)
}

func TestUpsertLocalMarketplaceEntry_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "alpha", "1.0.0", "test-repo"))
	require.NoError(t, upsertLocalMarketplaceEntry(path, "beta", "2.0.0", "test-repo"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 2)
}

func TestRemoveLocalMarketplaceEntry_RemovesEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertLocalMarketplaceEntry(path, "alpha", "1.0.0", "test-repo"))
	require.NoError(t, upsertLocalMarketplaceEntry(path, "beta", "2.0.0", "test-repo"))
	require.NoError(t, removeLocalMarketplaceEntry(path, "alpha"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "beta", m.Plugins[0].Name)
}

func TestRemoveLocalMarketplaceEntry_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	assert.NoError(t, removeLocalMarketplaceEntry(path, "any-plugin"))
	assert.NoFileExists(t, path, "remove on a missing file should not create the file")
}

func TestRemoveLocalMarketplaceEntry_MissingEntryIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")
	require.NoError(t, upsertLocalMarketplaceEntry(path, "alpha", "1.0.0", "test-repo"))
	require.NoError(t, removeLocalMarketplaceEntry(path, "nonexistent"))

	m := readMarketplaceFile(t, path)
	require.Len(t, m.Plugins, 1, "unrelated entries should be untouched")
}

func TestReadOrCreateLocalMarketplace_MissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	m, err := readOrCreateLocalMarketplace(path, "my-repo")
	require.NoError(t, err)
	assert.Equal(t, "my-repo", m.Name)
	assert.Equal(t, "JFrog", m.Owner.Name)
	assert.Empty(t, m.Plugins)
}

func TestReadOrCreateLocalMarketplace_ParsesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")
	require.NoError(t, upsertLocalMarketplaceEntry(path, "alpha", "1.0.0", "test-repo"))

	m, err := readOrCreateLocalMarketplace(path, "test-repo")
	require.NoError(t, err)
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "alpha", m.Plugins[0].Name)
}

func TestClaudeMarketplacePath(t *testing.T) {
	// After InjectRepoKey: installDir = ~/.claude/plugins/local/<repoKey>/<slug>
	// marketplace = ~/.claude/plugins/local/<repoKey>/.claude-plugin/marketplace.json
	installDir := filepath.Join("/home", "user", ".claude", "plugins", "local", "my-repo", "my-plugin")
	got := claudeMarketplacePath(installDir)
	assert.Equal(t, filepath.Join("/home", "user", ".claude", "plugins", "local", "my-repo", ".claude-plugin", "marketplace.json"), got)
}

func TestClaudeMarketplaceDir(t *testing.T) {
	installDir := filepath.Join("/home", "user", ".claude", "plugins", "local", "my-repo", "my-plugin")
	got := claudeMarketplaceDir(installDir)
	assert.Equal(t, filepath.Join("/home", "user", ".claude", "plugins", "local", "my-repo"), got)
}

func TestCodexMarketplaceManifestPath(t *testing.T) {
	// After InjectRepoKey: installDir = ~/.agents/plugins/local/<repoKey>/<slug>
	// root     = ~/.agents/plugins/local/<repoKey>
	// manifest = ~/.agents/plugins/local/<repoKey>/.agents/plugins/marketplace.json
	installDir := filepath.Join("/home", "user", ".agents", "plugins", "local", "my-repo", "my-plugin")
	got := codexMarketplaceManifestPath(installDir)
	assert.Equal(t, filepath.Join("/home", "user", ".agents", "plugins", "local", "my-repo", ".agents", "plugins", "marketplace.json"), got)
}

func TestCodexMarketplaceRoot(t *testing.T) {
	installDir := filepath.Join("/home", "user", ".agents", "plugins", "local", "my-repo", "my-plugin")
	got := codexMarketplaceRoot(installDir)
	assert.Equal(t, filepath.Join("/home", "user", ".agents", "plugins", "local", "my-repo"), got)
}

func TestUpsertCodexMarketplaceEntry_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")

	require.NoError(t, upsertCodexMarketplaceEntry(path, "my-plugin", "my-repo"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m codexMarketplace
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "my-repo", m.Name)
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "my-plugin", m.Plugins[0].Name)
	assert.Equal(t, "local", m.Plugins[0].Source.Source)
	assert.Equal(t, "./my-plugin", m.Plugins[0].Source.Path)
}

func TestRemoveCodexMarketplaceEntry_RemovesEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marketplace.json")
	require.NoError(t, upsertCodexMarketplaceEntry(path, "alpha", "my-repo"))
	require.NoError(t, upsertCodexMarketplaceEntry(path, "beta", "my-repo"))
	require.NoError(t, removeCodexMarketplaceEntry(path, "alpha"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m codexMarketplace
	require.NoError(t, json.Unmarshal(data, &m))
	require.Len(t, m.Plugins, 1)
	assert.Equal(t, "beta", m.Plugins[0].Name)
}

// readMarketplaceFile is a test helper that parses the JSON at path.
func readMarketplaceFile(t *testing.T, path string) localMarketplace {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test helper reading a known test path
	require.NoError(t, err)
	var m localMarketplace
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}
