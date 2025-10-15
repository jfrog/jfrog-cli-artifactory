package aieditorextensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUpdateMode(t *testing.T) {
	// Create temporary settings.json
	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create initial settings
	initialSettings := map[string]interface{}{
		"editor.fontSize": 14,
		"files.autoSave":  "afterDelay",
	}
	data, err := json.MarshalIndent(initialSettings, "", "    ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, data, 0644))

	// Test setting update mode
	settings, err := readSettings(settingsPath)
	require.NoError(t, err)

	settings["update.mode"] = "manual"

	err = writeSettings(settingsPath, settings)
	require.NoError(t, err)

	// Verify
	updated, err := readSettings(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, "manual", updated["update.mode"])
	assert.Equal(t, float64(14), updated["editor.fontSize"]) // JSON numbers are float64
	assert.Equal(t, "afterDelay", updated["files.autoSave"])
}

func TestReadWriteSettings(t *testing.T) {
	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create initial file (so backup can be created on next write)
	initialSettings := map[string]interface{}{
		"editor.fontSize": 12,
	}
	initialData, err := json.Marshal(initialSettings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, initialData, 0644))

	// Write settings (this should create backup)
	settings := map[string]interface{}{
		"editor.fontSize": 16,
		"update.mode":     "none",
		"files.autoSave":  "onFocusChange",
	}

	err = writeSettings(settingsPath, settings)
	require.NoError(t, err)

	// Read settings
	read, err := readSettings(settingsPath)
	require.NoError(t, err)

	assert.Equal(t, float64(16), read["editor.fontSize"])
	assert.Equal(t, "none", read["update.mode"])
	assert.Equal(t, "onFocusChange", read["files.autoSave"])

	// Verify backup was created
	backupPath := settingsPath + ".backup"
	assert.FileExists(t, backupPath)

	// Verify backup has original content
	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	var backupSettings map[string]interface{}
	require.NoError(t, json.Unmarshal(backupData, &backupSettings))
	assert.Equal(t, float64(12), backupSettings["editor.fontSize"])
}

func TestUpdateModeConstants(t *testing.T) {
	assert.Equal(t, UpdateMode("default"), UpdateModeDefault)
	assert.Equal(t, UpdateMode("manual"), UpdateModeManual)
	assert.Equal(t, UpdateMode("none"), UpdateModeNone)
}
