package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	jfrogSkillDirName = ".jfrog"

	// skillInfoJSON is the CLI manifest filename under .jfrog/.
	skillInfoJSON = "skill-info.json"
)

// SkillInfoManifestSchemaVersion is bumped when the JSON shape changes incompatibly.
const SkillInfoManifestSchemaVersion = 1

// SkillInfoManifest is CLI-owned metadata for an installed skill (single source of truth for list/update).
type SkillInfoManifest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Repo             string `json:"repo"`
	Slug             string `json:"slug"`
	InstalledVersion string `json:"installedVersion"`
	Scope            string `json:"scope"`
	Agent            string `json:"agent"`
	ProjectDir       string `json:"projectDir,omitempty"`
}

// SkillInfoManifestPath returns the path to the manifest under a skill install directory (.jfrog/skill-info.json).
func SkillInfoManifestPath(skillDir string) string {
	return filepath.Join(skillDir, jfrogSkillDirName, skillInfoJSON)
}

// WriteSkillInfoManifest writes the manifest under skillDir (.jfrog/skill-info.json).
func WriteSkillInfoManifest(skillDir string, manifest SkillInfoManifest) error {
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = SkillInfoManifestSchemaVersion
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skill info manifest: %w", err)
	}
	dir := filepath.Join(skillDir, jfrogSkillDirName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create .jfrog under skill dir: %w", err)
	}
	path := filepath.Join(dir, skillInfoJSON)
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write skill info manifest: %w", err)
	}
	return nil
}

// ReadSkillInfoManifest reads .jfrog/skill-info.json when present.
// A missing file returns (nil, nil).
func ReadSkillInfoManifest(skillDir string) (*SkillInfoManifest, error) {
	path := SkillInfoManifestPath(skillDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skill manifest: %w", err)
	}
	var manifest SkillInfoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse skill manifest %s: %w", path, err)
	}
	return &manifest, nil
}
