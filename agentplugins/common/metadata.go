package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultPluginVersion is used when no plugin.json declares a version and the user did
// not pass --version.
const DefaultPluginVersion = "1.0.0"

// KnownManifestRelPaths lists the fixed relative locations checked for plugin.json,
// in stable order. Discovery does not "first match wins" — every existing file is
// collected and validated for consistency.
var KnownManifestRelPaths = []string{
	"plugin.json",
	".cursor-plugin/plugin.json",
	".claude-plugin/plugin.json",
	".codex-plugin/plugin.json",
	".github/plugin/plugin.json",
	".plugin/plugin.json",
}

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var versionSafeRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-+]*$`)

// PluginMeta is the portable subset of plugin.json used for publish.
type PluginMeta struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type pluginJSON struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DiscoverPluginManifests returns a map of relative-path -> PluginMeta for each
// known manifest path that exists under pluginRoot. The map preserves the canonical
// relative path strings from KnownManifestRelPaths.
func DiscoverPluginManifests(pluginRoot string) (map[string]PluginMeta, error) {
	found := make(map[string]PluginMeta)
	for _, rel := range KnownManifestRelPaths {
		fullPath := filepath.Join(pluginRoot, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to stat %s: %w", rel, err)
		}
		if info.IsDir() {
			continue
		}
		meta, err := readPluginManifest(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", rel, err)
		}
		found[rel] = meta
	}
	return found, nil
}

func readPluginManifest(path string) (PluginMeta, error) {
	// #nosec G304 -- path is constructed by joining a user-provided directory with a fixed allowlist.
	data, err := os.ReadFile(path)
	if err != nil {
		return PluginMeta{}, err
	}
	var raw pluginJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return PluginMeta{}, err
	}
	return PluginMeta{
		Name:    strings.TrimSpace(raw.Name),
		Version: strings.TrimSpace(raw.Version),
	}, nil
}

// ValidateAndResolvePluginMeta discovers every plugin.json under pluginRoot, validates
// that all discovered manifests agree on name and version, and resolves the final
// publish identity using this precedence:
//
//  1. versionFlag (--version) overrides everything when non-empty
//  2. Consensus version from manifests, if non-empty
//  3. DefaultPluginVersion ("1.0.0")
func ValidateAndResolvePluginMeta(pluginRoot, versionFlag string) (PluginMeta, error) {
	manifests, err := DiscoverPluginManifests(pluginRoot)
	if err != nil {
		return PluginMeta{}, err
	}
	if len(manifests) == 0 {
		return PluginMeta{}, fmt.Errorf(
			"no plugin.json found under %s (checked: %s)",
			pluginRoot,
			strings.Join(KnownManifestRelPaths, ", "),
		)
	}

	var (
		canonicalName    string
		canonicalVersion string
		nameSource       string
		versionSource    string
	)
	for _, rel := range KnownManifestRelPaths {
		meta, ok := manifests[rel]
		if !ok {
			continue
		}
		if meta.Name == "" {
			return PluginMeta{}, fmt.Errorf("%s is missing required 'name' field", rel)
		}
		if canonicalName == "" {
			canonicalName = meta.Name
			nameSource = rel
		} else if meta.Name != canonicalName {
			return PluginMeta{}, fmt.Errorf(
				"plugin name mismatch: %s declares name=%q but %s declares name=%q",
				nameSource, canonicalName, rel, meta.Name,
			)
		}
		if meta.Version == "" {
			continue
		}
		if canonicalVersion == "" {
			canonicalVersion = meta.Version
			versionSource = rel
		} else if meta.Version != canonicalVersion {
			return PluginMeta{}, fmt.Errorf(
				"plugin version mismatch: %s declares version=%q but %s declares version=%q",
				versionSource, canonicalVersion, rel, meta.Version,
			)
		}
	}

	resolvedVersion := strings.TrimSpace(versionFlag)
	if resolvedVersion == "" {
		resolvedVersion = canonicalVersion
	}
	if resolvedVersion == "" {
		resolvedVersion = DefaultPluginVersion
	}

	return PluginMeta{Name: canonicalName, Version: resolvedVersion}, nil
}

// ValidateSlug checks that a plugin slug matches the required pattern.
func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("invalid plugin slug '%s': must match pattern ^[a-z0-9][a-z0-9-]*$", slug)
	}
	return nil
}

// ValidateVersion checks that a version string is safe for use in file paths.
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version must not be empty")
	}
	if strings.Contains(version, "..") {
		return fmt.Errorf("invalid version '%s': must not contain '..'", version)
	}
	if !versionSafeRegex.MatchString(version) {
		return fmt.Errorf("invalid version '%s': must contain only alphanumeric characters, dots, hyphens, and plus signs", version)
	}
	return nil
}
