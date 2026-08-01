package python

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertOwnerOnly verifies Unix owner-only permission bits. Windows has no
// equivalent: os.Chmod there only toggles the read-only attribute, so the mode
// always reads back as 0666.
func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	if coreutils.IsWindows() {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

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

	assertOwnerOnly(t, customConfigPath)
}

func TestResolvePipConfigPath_PIP_CONFIG_FILE(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-pip.conf")
	t.Setenv("PIP_CONFIG_FILE", custom)

	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(custom), got)
}

// An existing config left at 0644 by a pre-fix run must be tightened too:
// os.WriteFile applies its mode only when it creates the file.
func TestCreatePipConfigManually_HardensExistingFile(t *testing.T) {
	customConfigPath := filepath.Join(t.TempDir(), "pip.conf")
	require.NoError(t, os.WriteFile(customConfigPath, []byte("stale\n"), 0644))
	require.NoError(t, os.Chmod(customConfigPath, 0644))

	// #nosec G101 -- False positive - no hardcoded credentials.
	require.NoError(t, CreatePipConfigManually(customConfigPath, "https://example.com/simple/"))

	assertOwnerOnly(t, customConfigPath)
}

// pip prefers ~/Library/Application Support/pip when that directory exists, so
// the derived path must follow it there rather than assuming ~/.config.
func TestResolvePipConfigPath_MacOSApplicationSupport(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific pip config resolution")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PIP_CONFIG_FILE", "")

	// Absent the directory, pip falls back to the Linux-like path.
	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "pip", "pip.conf"), got)

	require.NoError(t, os.MkdirAll(filepath.Join(home, "Library", "Application Support", "pip"), 0700))
	got, err = ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "Library", "Application Support", "pip", "pip.conf"), got)
}

// pip's darwin branch hard-codes ~/.config and never consults XDG_CONFIG_HOME.
func TestResolvePipConfigPath_MacOSIgnoresXdg(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific pip config resolution")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PIP_CONFIG_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "pip", "pip.conf"), got)
}

func TestResolvePipConfigPath_XdgConfigHome(t *testing.T) {
	if coreutils.IsWindows() || runtime.GOOS == "darwin" {
		t.Skip("XDG_CONFIG_HOME applies to non-macOS Unix only")
	}
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("PIP_CONFIG_FILE", "")

	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "pip", "pip.conf"), got)

	t.Setenv("XDG_CONFIG_HOME", "")
	got, err = ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "pip", "pip.conf"), got)
}

func TestResolvePipConfigPath_WindowsAppData(t *testing.T) {
	if !coreutils.IsWindows() {
		t.Skip("Windows-specific pip config resolution")
	}
	appData := t.TempDir()
	t.Setenv("PIP_CONFIG_FILE", "")
	t.Setenv("APPDATA", appData)

	got, err := ResolvePipConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(appData, "pip", "pip.ini"), got)

	// os.UserConfigDir surfaces the unset-%AppData% case for us.
	t.Setenv("APPDATA", "")
	_, err = ResolvePipConfigPath()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "appdata")
}

// HardenPipConfigPermissions tightens the user-level pip config the derived path
// resolves to (PIP_CONFIG_FILE here) from 0644 to owner-only.
func TestHardenPipConfigPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pip")
	require.NoError(t, os.MkdirAll(dir, 0700))
	conf := filepath.Join(dir, "pip.conf")
	require.NoError(t, os.WriteFile(conf, []byte("[global]\nindex-url = https://x\n"), 0644))
	t.Setenv("PIP_CONFIG_FILE", conf)

	HardenPipConfigPermissions()

	assertOwnerOnly(t, conf)
}

// Best-effort: a config file the derived path cannot locate is warned about, not
// fatal - our derivation can disagree with pip, and failing would break
// `jf setup pip` on a machine it just configured correctly.
func TestHardenPipConfigPermissions_MissingFileDoesNotPanic(t *testing.T) {
	t.Setenv("PIP_CONFIG_FILE", filepath.Join(t.TempDir(), "does-not-exist.conf"))

	assert.NotPanics(t, HardenPipConfigPermissions)
}
