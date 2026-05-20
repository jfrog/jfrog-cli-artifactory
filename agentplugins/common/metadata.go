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

// pluginJSONVersionRegex matches a string "version" field in plugin.json without reformatting the file.
var pluginJSONVersionRegex = regexp.MustCompile(`("version"\s*:\s*")([^"]*)(")`)

// PluginMeta is the portable subset of plugin.json used for publish.
// When read from a single file, only Name and Version are set. After
// ValidateAndResolvePluginMeta, Version is the final publish version and
// ManifestVersion holds the on-disk consensus before --version.
type PluginMeta struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	// ManifestVersion is the consensus version from plugin.json files only (before --version).
	ManifestVersion string `json:"-"`
}

// DiscoverPluginManifests returns a map of relative-path -> PluginMeta for each
// known manifest path that exists under pluginRoot. The map preserves the canonical
// relative path strings from KnownManifestRelPaths.
func DiscoverPluginManifests(pluginRoot string) (map[string]PluginMeta, error) {
	found := make(map[string]PluginMeta)
	for _, relativePath := range KnownManifestRelPaths {
		fullPath := filepath.Join(pluginRoot, relativePath)
		info, err := os.Stat(fullPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to stat %s: %w", relativePath, err)
		}
		if info.IsDir() {
			continue
		}
		meta, err := readPluginManifest(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", relativePath, err)
		}
		found[relativePath] = meta
	}
	return found, nil
}

func readPluginManifest(path string) (PluginMeta, error) {
	// #nosec G304 -- path is constructed by joining a user-provided directory with a fixed allowlist.
	data, err := os.ReadFile(path)
	if err != nil {
		return PluginMeta{}, err
	}
	var meta PluginMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return PluginMeta{}, err
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Version = strings.TrimSpace(meta.Version)
	return meta, nil
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
	for _, relativePath := range KnownManifestRelPaths {
		meta, ok := manifests[relativePath]
		if !ok {
			continue
		}
		if meta.Name == "" {
			return PluginMeta{}, fmt.Errorf("%s is missing required 'name' field", relativePath)
		}
		if canonicalName == "" {
			canonicalName = meta.Name
			nameSource = relativePath
		} else if meta.Name != canonicalName {
			return PluginMeta{}, fmt.Errorf(
				"plugin name mismatch: %s declares name=%q but %s declares name=%q",
				nameSource, canonicalName, relativePath, meta.Name,
			)
		}
		if meta.Version == "" {
			continue
		}
		if canonicalVersion == "" {
			canonicalVersion = meta.Version
			versionSource = relativePath
		} else if meta.Version != canonicalVersion {
			return PluginMeta{}, fmt.Errorf(
				"plugin version mismatch: %s declares version=%q but %s declares version=%q",
				versionSource, canonicalVersion, relativePath, meta.Version,
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

	return PluginMeta{
		Name:            canonicalName,
		Version:         resolvedVersion,
		ManifestVersion: canonicalVersion,
	}, nil
}

// UpdatePluginManifestVersions rewrites the version field in every discovered plugin.json
// that already declares a non-empty version. Manifests without a version field are unchanged,
// matching jf skills publish behavior for SKILL.md.
func UpdatePluginManifestVersions(pluginRoot, newVersion string) error {
	manifests, err := DiscoverPluginManifests(pluginRoot)
	if err != nil {
		return err
	}
	for _, relativePath := range KnownManifestRelPaths {
		meta, ok := manifests[relativePath]
		if !ok || meta.Version == "" || meta.Version == newVersion {
			continue
		}
		fullPath := filepath.Join(pluginRoot, relativePath)
		if err := writePluginManifestVersion(fullPath, newVersion); err != nil {
			return fmt.Errorf("%s: %w", relativePath, err)
		}
	}
	return nil
}

func writePluginManifestVersion(path, newVersion string) error {
	// #nosec G304 -- path is constructed from pluginRoot and KnownManifestRelPaths allowlist.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var meta PluginMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	if strings.TrimSpace(meta.Version) == "" {
		return nil
	}
	content := string(data)
	if !pluginJSONVersionRegex.MatchString(content) {
		return fmt.Errorf("plugin.json declares version %q but has no replaceable \"version\" string field", meta.Version)
	}
	updated := pluginJSONVersionRegex.ReplaceAllString(content, `${1}`+newVersion+`${3}`)
	// #nosec G306,G703 -- path is pluginRoot + KnownManifestRelPaths allowlist; user-owned manifest.
	return os.WriteFile(path, []byte(updated), 0o600)
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
