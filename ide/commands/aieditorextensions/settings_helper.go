package aieditorextensions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

// UpdateMode represents VSCode update modes
type UpdateMode string

const (
	UpdateModeDefault UpdateMode = "default" // Auto-update (VSCode default)
	UpdateModeManual  UpdateMode = "manual"  // Prompt for updates
	UpdateModeNone    UpdateMode = "none"    // Disable updates
)

// SetUpdateMode sets the VSCode-based IDE update mode in user settings.json
func SetUpdateMode(mode UpdateMode, settingsDir string) error {
	settingsPath, err := detectSettingsPath(settingsDir)
	if err != nil {
		return fmt.Errorf("failed to locate settings.json: %w", err)
	}

	log.Info("Configuring IDE update mode in settings.json...")

	// Read existing settings
	settings, err := readSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	// Set update.mode
	settings["update.mode"] = string(mode)

	// Write back
	if err := writeSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	log.Info("Update mode set to:", string(mode))
	log.Info("Configuration file:", settingsPath)

	return nil
}

// detectSettingsPath finds the IDE's settings.json file based on the settings directory name
func detectSettingsPath(settingsDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	var possiblePaths []string

	// Platform-specific paths
	switch runtime.GOOS {
	case "darwin":
		possiblePaths = []string{
			filepath.Join(homeDir, "Library", "Application Support", settingsDir, "User", "settings.json"),
		}
	case "linux":
		possiblePaths = []string{
			filepath.Join(homeDir, ".config", settingsDir, "User", "settings.json"),
		}
	case "windows":
		possiblePaths = []string{
			filepath.Join(homeDir, "AppData", "Roaming", settingsDir, "User", "settings.json"),
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Check each path
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// If no settings.json exists, create in default location
	defaultPath := possiblePaths[0]
	dir := filepath.Dir(defaultPath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create settings directory: %w", err)
	}

	// Create empty settings.json
	if err := os.WriteFile(defaultPath, []byte("{\n}\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to create settings.json: %w", err)
	}

	log.Info("Created new settings.json at:", defaultPath)
	return defaultPath, nil
}

// readSettings reads and parses settings.json
func readSettings(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// writeSettings writes settings.json with pretty formatting
func writeSettings(path string, settings map[string]interface{}) error {
	// Create backup
	backupPath := path + ".backup"
	if data, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			log.Debug("Warning: failed to create backup:", err)
		}
	}

	// Write with pretty formatting
	data, err := json.MarshalIndent(settings, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
