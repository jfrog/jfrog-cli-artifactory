package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

// localMarketplaceOwner identifies the publisher of a JFrog-managed marketplace.
type localMarketplaceOwner struct {
	Name string `json:"name"`
}

// localMarketplace is the on-disk shape of the JFrog-managed local marketplace.json.
// The owner field is required by the Claude plugin schema; Codex accepts it too.
type localMarketplace struct {
	Name    string                  `json:"name"`
	Owner   localMarketplaceOwner   `json:"owner"`
	Plugins []localMarketplaceEntry `json:"plugins"`
}

// localMarketplaceEntry is a single plugin record in the local marketplace.json.
type localMarketplaceEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Source is a relative path from the marketplace root to the plugin directory
	// (e.g. "./my-plugin"). Both Claude and Codex accept a plain string here.
	Source string `json:"source"`
}

// upsertLocalMarketplaceEntry reads or creates the marketplace JSON at path,
// adds or replaces the entry for slug, then writes it back.
// marketplaceName is written as the manifest's "name" field (typically the Artifactory repo key).
// source is set to "./<slug>" — a path relative to the marketplace root directory.
func upsertLocalMarketplaceEntry(path, slug, version, marketplaceName string) error {
	m, err := readOrCreateLocalMarketplace(path, marketplaceName)
	if err != nil {
		return err
	}
	entry := localMarketplaceEntry{
		Name:    slug,
		Version: version,
		Source:  "./" + slug,
	}
	found := false
	for i, p := range m.Plugins {
		if strings.EqualFold(p.Name, slug) {
			m.Plugins[i] = entry
			found = true
			break
		}
	}
	if !found {
		m.Plugins = append(m.Plugins, entry)
	}
	return writeLocalMarketplace(path, m)
}

// removeLocalMarketplaceEntry reads the marketplace JSON at path, removes the
// entry for slug, and writes it back only if the slug was present.
// A missing file or a missing slug entry is a no-op.
func removeLocalMarketplaceEntry(path, slug string) error {
	m, err := readOrCreateLocalMarketplace(path, "")
	if err != nil {
		return err
	}
	n := len(m.Plugins)
	kept := m.Plugins[:0]
	for _, p := range m.Plugins {
		if !strings.EqualFold(p.Name, slug) {
			kept = append(kept, p)
		}
	}
	if len(kept) == n {
		return nil // slug not present; nothing to write
	}
	m.Plugins = kept
	return writeLocalMarketplace(path, m)
}

// readOrCreateLocalMarketplace reads and parses path. When the file does not exist,
// it returns an empty marketplace with the given name.
func readOrCreateLocalMarketplace(path, marketplaceName string) (*localMarketplace, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is tool-managed config under agent home
	if err != nil {
		if os.IsNotExist(err) {
			return &localMarketplace{
				Name:    marketplaceName,
				Owner:   localMarketplaceOwner{Name: "JFrog"},
				Plugins: []localMarketplaceEntry{},
			}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m localMarketplace
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Plugins == nil {
		m.Plugins = []localMarketplaceEntry{}
	}
	return &m, nil
}

// writeLocalMarketplace creates parent directories as needed and writes m to path.
func writeLocalMarketplace(path string, m *localMarketplace) error {
	if err := os.MkdirAll(filepath.Dir(path), agentcommon.InstallDirMode); err != nil {
		return fmt.Errorf("create dirs for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marketplace: %w", err)
	}
	if err := os.WriteFile(path, data, agentcommon.InstallManifestFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
