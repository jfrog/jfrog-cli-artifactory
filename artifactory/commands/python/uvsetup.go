package python

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	uvIndexName = "fly-pypi"
)

type uvIndex struct {
	Name    string `toml:"name"`
	URL     string `toml:"url"`
	Default bool   `toml:"default,omitempty"`
}

// RunUVAuthLogin stores credentials in UV's native credential store.
//
//	uv auth login <service-url> --username <user> --password <password>
func RunUVAuthLogin(serviceURL, username, password string) error {
	log.Debug("Running uv auth login for", serviceURL)
	cmd := exec.Command("uv", "auth", "login", serviceURL, "--username", username, "--password", password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("uv auth login failed: %s\n%s", err, string(output))
	}
	return nil
}

// RunUVAuthLogout removes credentials from UV's native credential store.
//
//	uv auth logout <service-url> --username <user>
func RunUVAuthLogout(serviceURL, username string) error {
	log.Debug("Running uv auth logout for", serviceURL)
	args := []string{"auth", "logout", serviceURL}
	if username != "" {
		args = append(args, "--username", username)
	}
	cmd := exec.Command("uv", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("uv auth logout failed: %s\n%s", err, string(output))
	}
	return nil
}

// ConfigureUVIndex writes a [[index]] entry to the user-level uv.toml.
// If the file already exists, it adds or updates the entry with the given name
// while preserving all other settings in the file.
func ConfigureUVIndex(indexURL string) error {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return err
	}

	fullCfg, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return err
	}

	found := false
	for i, idx := range indexes {
		if idx.Name == uvIndexName {
			indexes[i].URL = indexURL
			indexes[i].Default = true
			found = true
			break
		}
	}
	if !found {
		indexes = append(indexes, uvIndex{
			Name:    uvIndexName,
			URL:     indexURL,
			Default: true,
		})
	}

	return writeUVConfig(configPath, fullCfg, indexes)
}

// RemoveUVIndex removes the Fly index entry from the user-level uv.toml.
// If the config file doesn't exist, this is a no-op.
func RemoveUVIndex() error {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	fullCfg, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return err
	}

	filtered := make([]uvIndex, 0, len(indexes))
	for _, idx := range indexes {
		if idx.Name != uvIndexName {
			filtered = append(filtered, idx)
		}
	}

	return writeUVConfig(configPath, fullCfg, filtered)
}

// GetConfiguredUVIndexURL reads the user-level uv.toml and returns the URL
// for the Fly index entry, or empty string if not found.
func GetConfiguredUVIndexURL() (string, error) {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return "", err
	}

	_, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return "", err
	}

	for _, idx := range indexes {
		if idx.Name == uvIndexName {
			return idx.URL, nil
		}
	}
	return "", nil
}

func getUserUVConfigPath() (string, error) {
	if configFile := os.Getenv("UV_CONFIG_FILE"); configFile != "" {
		return configFile, nil
	}

	var configDir string
	if runtime.GOOS == "windows" {
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			return "", errorutils.CheckErrorf("%%APPDATA%% not set")
		}
		configDir = filepath.Join(configDir, "uv")
	} else {
		configDir = os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", errorutils.CheckErrorf("failed to determine home directory: %w", err)
			}
			configDir = filepath.Join(home, ".config", "uv")
		} else {
			configDir = filepath.Join(configDir, "uv")
		}
	}
	return filepath.Join(configDir, "uv.toml"), nil
}

// loadUVConfig reads the uv.toml at path.
// Returns the full config as a generic map (to preserve unknown keys on write)
// and the parsed [[index]] entries separately.
func loadUVConfig(path string) (map[string]any, []uvIndex, error) {
	fullCfg := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fullCfg, nil, nil
		}
		return nil, nil, errorutils.CheckErrorf("failed to read uv config at %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &fullCfg); err != nil {
		return nil, nil, errorutils.CheckErrorf("failed to parse uv config at %s: %w", path, err)
	}

	// Extract [[index]] entries into typed structs
	var indexes []uvIndex
	if rawIndexes, ok := fullCfg["index"]; ok {
		indexSlice, ok := rawIndexes.([]map[string]any)
		if !ok {
			return fullCfg, nil, nil
		}
		for _, entry := range indexSlice {
			idx := uvIndex{}
			if name, ok := entry["name"].(string); ok {
				idx.Name = name
			}
			if url, ok := entry["url"].(string); ok {
				idx.URL = url
			}
			if def, ok := entry["default"].(bool); ok {
				idx.Default = def
			}
			indexes = append(indexes, idx)
		}
	}

	return fullCfg, indexes, nil
}

// writeUVConfig writes the full config map back to disk, replacing the "index"
// key with the provided entries while preserving all other settings.
func writeUVConfig(path string, fullCfg map[string]any, indexes []uvIndex) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return errorutils.CheckErrorf("failed to create uv config directory: %w", err)
	}

	// Convert typed indexes back to generic maps for TOML encoding
	indexMaps := make([]map[string]any, 0, len(indexes))
	for _, idx := range indexes {
		m := map[string]any{
			"name": idx.Name,
			"url":  idx.URL,
		}
		if idx.Default {
			m["default"] = true
		}
		indexMaps = append(indexMaps, m)
	}

	if len(indexMaps) > 0 {
		fullCfg["index"] = indexMaps
	} else {
		delete(fullCfg, "index")
	}

	var b strings.Builder
	encoder := toml.NewEncoder(&b)
	if err := encoder.Encode(fullCfg); err != nil {
		return errorutils.CheckErrorf("failed to encode uv config: %w", err)
	}

	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return fmt.Errorf("failed to write uv config at %s: %w", path, err)
	}

	log.Debug("Wrote uv configuration to", path)
	return nil
}
