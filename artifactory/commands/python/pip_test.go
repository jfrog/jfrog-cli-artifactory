package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePipConfigManually(t *testing.T) {
	// Define the test parameters
	customConfigPath := filepath.Join(t.TempDir(), "tmp", "test", "pip.conf")
	// #nosec G101 -- False positive - no hardcoded credentials.
	repoWithCredsUrl := "https://example.com/simple/"
	expectedContent := "[global]\nindex-url = https://example.com/simple/\n"

	// Call the function under test
	err := CreatePipConfigManually(customConfigPath, repoWithCredsUrl)

	// Assert no error occurred
	assert.NoError(t, err)

	// Verify the file exists and has the correct content
	fileContent, err := os.ReadFile(customConfigPath)
	assert.NoError(t, err)
	assert.Equal(t, expectedContent, string(fileContent))

	info, err := os.Stat(customConfigPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestResolvePipConfigPath_PIP_CONFIG_FILE(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-pip.conf")
	t.Setenv("PIP_CONFIG_FILE", custom)

	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(custom), got)
}

func TestHardenPipConfigPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pip")
	require.NoError(t, os.MkdirAll(dir, 0700))
	conf := filepath.Join(dir, "pip.conf")
	require.NoError(t, os.WriteFile(conf, []byte("[global]\nindex-url = https://x\n"), 0644))
	t.Setenv("PIP_CONFIG_FILE", conf)

	require.NoError(t, HardenPipConfigPermissions())

	info, err := os.Stat(conf)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestHardenPipConfigPermissions_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.conf")
	t.Setenv("PIP_CONFIG_FILE", missing)

	err := HardenPipConfigPermissions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}
