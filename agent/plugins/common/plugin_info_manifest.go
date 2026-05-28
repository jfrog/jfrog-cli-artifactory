package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

const (
	jfrogPluginDirName = ".jfrog"

	// pluginInfoJSON is the CLI manifest filename under .jfrog/.
	pluginInfoJSON = "plugin-info.json"
)

// PluginInfoManifestSchemaVersion is bumped when the JSON shape changes incompatibly.
const PluginInfoManifestSchemaVersion = 1

// PluginInfoManifest is CLI-owned metadata for an installed plugin (single source of truth for list/update).
type PluginInfoManifest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Repo             string `json:"repo"`
	Slug             string `json:"slug"`
	InstalledVersion string `json:"installedVersion"`
	Scope            string `json:"scope"`
	Agent            string `json:"agent"`
	ProjectDir       string `json:"projectDir,omitempty"`
}

// PluginInfoManifestPath returns the path to the manifest under a plugin install directory (.jfrog/plugin-info.json).
func PluginInfoManifestPath(pluginDir string) string {
	return filepath.Join(pluginDir, jfrogPluginDirName, pluginInfoJSON)
}

// WritePluginInfoManifest writes the manifest under pluginDir (.jfrog/plugin-info.json).
func WritePluginInfoManifest(pluginDir string, manifest PluginInfoManifest) error {
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = PluginInfoManifestSchemaVersion
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugin info manifest: %w", err)
	}
	dir := filepath.Join(pluginDir, jfrogPluginDirName)
	if err := os.MkdirAll(dir, agentcommon.InstallDirMode); err != nil {
		return fmt.Errorf("create .jfrog under plugin dir: %w", err)
	}
	path := filepath.Join(dir, pluginInfoJSON)
	// #nosec G306 -- manifest lives under user-owned plugin dir; mode matches install/list expectations for CLI metadata.
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write plugin info manifest: %w", err)
	}
	return nil
}

// ReadPluginInfoManifest reads .jfrog/plugin-info.json when present.
// A missing file returns (nil, nil).
func ReadPluginInfoManifest(pluginDir string) (*PluginInfoManifest, error) {
	path := PluginInfoManifestPath(pluginDir)
	// #nosec G304 -- path is plugin install directory joined with fixed .jfrog/plugin-info.json segments.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugin manifest: %w", err)
	}
	var manifest PluginInfoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse plugin manifest %s: %w", path, err)
	}
	return &manifest, nil
}
