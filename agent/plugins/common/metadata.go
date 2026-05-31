package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

// manifestFileName is the canonical plugin manifest filename at the plugin root.
const manifestFileName = "plugin.json"

// manifestVersionField is the top-level JSON key for the publish version in plugin.json.
const manifestVersionField = "version"

// manifestJSONIndent is used when rewriting plugin.json so the file stays human-readable.
const manifestJSONIndent = "    "

// defaultPluginVersion is used when no plugin.json declares a version and the user did
// not pass --version.
const defaultPluginVersion = "1.0.0"

// knownManifestRelPaths lists the built-in relative locations checked for plugin.json.
// agent-config.json "plugin-manifest-paths" entries are prepended (higher priority);
// defaults fill in any path not already listed. The first existing file wins.
var knownManifestRelPaths = []string{
	".claude-plugin/" + manifestFileName,
	".cursor-plugin/" + manifestFileName,
	".codex-plugin/" + manifestFileName,
	manifestFileName,
	".github/plugin/" + manifestFileName,
	".plugin/" + manifestFileName,
}

// loadPluginManifestPaths returns plugin.json search paths for publish.
// agent-config.json "plugin-manifest-paths" come first; knownManifestRelPaths follow,
// skipping duplicates while preserving order.
func loadPluginManifestPaths() ([]string, error) {
	section, path, err := agentcommon.LoadAgentConfigSection(agentcommon.PluginManifestPathsKey)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return append([]string(nil), knownManifestRelPaths...), nil
	}
	var fromConfig []string
	if err := json.Unmarshal(section, &fromConfig); err != nil {
		return nil, fmt.Errorf("failed to parse %q in %s: %w", agentcommon.PluginManifestPathsKey, path, err)
	}
	return mergePluginManifestPaths(fromConfig), nil
}

// mergePluginManifestPaths prepends config paths, then appends built-in defaults once each.
// addedPaths records relative manifest paths already in the result (dedup by path string).
func mergePluginManifestPaths(fromConfig []string) []string {
	addedPaths := make(map[string]struct{})
	orderedPaths := make([]string, 0, len(fromConfig)+len(knownManifestRelPaths))

	for _, relativePath := range fromConfig {
		relativePath = strings.TrimSpace(relativePath)
		if relativePath == "" {
			continue
		}
		if _, alreadyAdded := addedPaths[relativePath]; alreadyAdded {
			continue
		}
		addedPaths[relativePath] = struct{}{}
		orderedPaths = append(orderedPaths, relativePath)
	}
	for _, relativePath := range knownManifestRelPaths {
		if _, alreadyAdded := addedPaths[relativePath]; alreadyAdded {
			continue
		}
		addedPaths[relativePath] = struct{}{}
		orderedPaths = append(orderedPaths, relativePath)
	}
	return orderedPaths
}

// PluginMeta is the portable subset of plugin.json used for publish.
// When read from a single file, only Name and Version are set. After
// ValidateAndResolvePluginMeta, Version is the final publish version and
// ManifestVersion holds the on-disk consensus before --version.
type PluginMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// ManifestVersion is the consensus version from plugin.json files only (before --version).
	ManifestVersion string `json:"-"`
}

// findPrimaryPluginManifest returns the first plugin.json found under pluginRoot,
// searching loadPluginManifestPaths() in order.
func findPrimaryPluginManifest(pluginRoot string) (relativePath string, meta PluginMeta, err error) {
	relPaths, err := loadPluginManifestPaths()
	if err != nil {
		return "", PluginMeta{}, err
	}
	for _, relativePath := range relPaths {
		fullPath := filepath.Join(pluginRoot, relativePath)
		info, statErr := os.Stat(fullPath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return "", PluginMeta{}, fmt.Errorf("failed to stat %s: %w", relativePath, statErr)
		}
		if info.IsDir() {
			continue
		}
		meta, err := readPluginManifest(fullPath)
		if err != nil {
			return "", PluginMeta{}, fmt.Errorf("failed to parse %s: %w", relativePath, err)
		}
		return relativePath, meta, nil
	}
	return "", PluginMeta{}, pluginManifestNotFoundError(pluginRoot, relPaths)
}

func pluginManifestNotFoundError(pluginRoot string, relPaths []string) error {
	configPath := agentcommon.AgentConfigPathForDisplay()
	return fmt.Errorf(
		"no %s found under %s (checked: %s).\n\n"+
			"To search additional locations, edit %s and add relative paths under %q "+
			"(paths are relative to the plugin directory). Custom paths are checked first, "+
			"then built-in defaults.\n\n"+
			"Example:\n"+
			"  {\n"+
			"    %q: [\n"+
			"      \"my-layout/%s\"\n"+
			"    ]\n"+
			"  }",
		manifestFileName,
		pluginRoot,
		strings.Join(relPaths, ", "),
		configPath,
		agentcommon.PluginManifestPathsKey,
		agentcommon.PluginManifestPathsKey,
		manifestFileName,
	)
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

// ValidateAndResolvePluginMeta loads the first plugin.json under pluginRoot (see knownManifestRelPaths)
// and resolves the final publish identity using this precedence:
//
//  1. versionFlag (--version) overrides everything when non-empty
//  2. version from the canonical manifest, if non-empty
//  3. defaultPluginVersion ("1.0.0")
func ValidateAndResolvePluginMeta(pluginRoot, versionFlag string) (PluginMeta, error) {
	relativePath, meta, err := findPrimaryPluginManifest(pluginRoot)
	if err != nil {
		return PluginMeta{}, err
	}
	if meta.Name == "" {
		return PluginMeta{}, fmt.Errorf("%s is missing required 'name' field", relativePath)
	}

	manifestVersion := strings.TrimSpace(meta.Version)
	resolvedVersion := strings.TrimSpace(versionFlag)
	if resolvedVersion == "" {
		resolvedVersion = manifestVersion
	}
	if resolvedVersion == "" {
		resolvedVersion = defaultPluginVersion
	}

	return PluginMeta{
		Name:            meta.Name,
		Version:         resolvedVersion,
		ManifestVersion: manifestVersion,
	}, nil
}

// UpdatePluginManifestVersions rewrites the top-level "version" string field in the canonical
// plugin.json (first match in knownManifestRelPaths). Manifests without a version field are unchanged.
func UpdatePluginManifestVersions(pluginRoot, newVersion string) error {
	relativePath, meta, err := findPrimaryPluginManifest(pluginRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(meta.Version) == "" || strings.TrimSpace(meta.Version) == newVersion {
		return nil
	}
	fullPath := filepath.Join(pluginRoot, relativePath)
	if err := writePluginManifestVersion(fullPath, newVersion); err != nil {
		return fmt.Errorf("%s: %w", relativePath, err)
	}
	return nil
}

func writePluginManifestVersion(path, newVersion string) error {
	// #nosec G304 -- path is constructed from pluginRoot and knownManifestRelPaths allowlist.
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
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if _, hasVersionField := doc[manifestVersionField]; !hasVersionField {
		return fmt.Errorf("%s declares version %q but has no %q field", manifestFileName, meta.Version, manifestVersionField)
	}
	versionJSON, err := json.Marshal(newVersion)
	if err != nil {
		return err
	}
	doc[manifestVersionField] = versionJSON

	updated, err := json.MarshalIndent(doc, "", manifestJSONIndent)
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	// #nosec G306,G703 -- path is pluginRoot + knownManifestRelPaths allowlist; user-owned manifest.
	return os.WriteFile(path, updated, agentcommon.PrivateFileMode)
}

// ValidateSlug checks that a plugin slug is safe for repository paths and Artifactory layout.
// It must start with a lowercase letter or digit, then contain only lowercase letters, digits, or hyphens.
//
// Accepted examples: "my-plugin", "skill123", "a", "4chan-reader"
// Not accepted examples: "", "-invalid", "My-Skill", "has space", "foo/bar"
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("invalid plugin slug %q: must not be empty", slug)
	}
	if !isSlugStartChar(slug[0]) {
		return fmt.Errorf("invalid plugin slug %q: must start with a lowercase letter or digit", slug)
	}
	for charIndex := 1; charIndex < len(slug); charIndex++ {
		if !isSlugChar(slug[charIndex]) {
			return fmt.Errorf("invalid plugin slug %q: may contain only lowercase letters, digits, and hyphens", slug)
		}
	}
	return nil
}

func isSlugStartChar(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
}

func isSlugChar(character byte) bool {
	return isSlugStartChar(character) || character == '-'
}

// ValidateVersion checks that version is a valid semantic version for publish paths and Artifactory layout.
func ValidateVersion(version string) error {
	return agentcommon.ValidateSemver(version)
}
