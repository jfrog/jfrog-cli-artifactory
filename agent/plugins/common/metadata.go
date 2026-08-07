package common

import (
	"bytes"
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

// ErrPluginManifestNotFound is returned when no plugin.json could be located in any of
// the searched paths under a plugin directory.
var ErrPluginManifestNotFound = errors.New("no plugin.json")

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

// agentManifestRelPath maps a harness name to the manifest path convention jf publishes
// per-harness content under. Returns ok=false for harnesses with no such convention (e.g.
// custom agents from agent-config.json), which fall back to the default search order.
func agentManifestRelPath(agentName string) (string, bool) {
	switch strings.ToLower(agentName) {
	case "claude":
		return ".claude-plugin/" + manifestFileName, true
	case "codex":
		return ".codex-plugin/" + manifestFileName, true
	case "cursor":
		return ".cursor-plugin/" + manifestFileName, true
	default:
		return "", false
	}
}

// findPluginManifestForAgent behaves like findPrimaryPluginManifest, but tries agentName's
// own manifest convention first (e.g. .codex-plugin/plugin.json for codex) before falling
// back to the default search order. Without this, fields like Description would always come
// from .claude-plugin/plugin.json regardless of which harness is actually being read, since
// that's first in knownManifestRelPaths.
func findPluginManifestForAgent(pluginRoot, agentName string) (relativePath string, meta PluginMeta, err error) {
	if preferred, ok := agentManifestRelPath(agentName); ok {
		fullPath := filepath.Join(pluginRoot, preferred)
		if info, statErr := os.Stat(fullPath); statErr == nil && !info.IsDir() {
			meta, err := readPluginManifest(fullPath)
			if err != nil {
				return "", PluginMeta{}, fmt.Errorf("failed to parse %s: %w", preferred, err)
			}
			return preferred, meta, nil
		}
	}
	return findPrimaryPluginManifest(pluginRoot)
}

// findPrimaryPluginManifest returns the first plugin.json found under pluginRoot,
// searching loadPluginManifestPaths() in order.
func findPrimaryPluginManifest(pluginRoot string) (relativePath string, meta PluginMeta, err error) {
	manifestRelativePaths, err := loadPluginManifestPaths()
	if err != nil {
		return "", PluginMeta{}, err
	}
	for _, relativePath := range manifestRelativePaths {
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
	return "", PluginMeta{}, pluginManifestNotFoundError(pluginRoot, manifestRelativePaths)
}

func pluginManifestNotFoundError(pluginRoot string, manifestRelativePaths []string) error {
	configPath := agentcommon.AgentConfigPathForDisplay()
	return fmt.Errorf(
		"%w found under %s (checked: %s).\n\n"+
			"To search additional locations, edit %s and add relative paths under %q "+
			"(paths are relative to the plugin directory). Custom paths are checked first, "+
			"then built-in defaults.\n\n"+
			"Example:\n"+
			"  {\n"+
			"    %q: [\n"+
			"      \"my-layout/%s\"\n"+
			"    ]\n"+
			"  }",
		ErrPluginManifestNotFound,
		pluginRoot,
		strings.Join(manifestRelativePaths, ", "),
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

// UpdatePluginManifestVersions rewrites the top-level "version" string field in every plugin.json
// manifest found under pluginRoot (see loadPluginManifestPaths). Manifests without a version field,
// or already matching newVersion, are left unchanged. Returns an error if no manifest is found at all.
func UpdatePluginManifestVersions(pluginRoot, newVersion string) error {
	manifestRelativePaths, err := loadPluginManifestPaths()
	if err != nil {
		return err
	}
	found := false
	for _, relativePath := range manifestRelativePaths {
		fullPath := filepath.Join(pluginRoot, relativePath)
		info, statErr := os.Stat(fullPath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", relativePath, statErr)
		}
		if info.IsDir() {
			continue
		}
		found = true
		meta, err := readPluginManifest(fullPath)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", relativePath, err)
		}
		if strings.TrimSpace(meta.Version) == "" || strings.TrimSpace(meta.Version) == newVersion {
			continue
		}
		if err := writePluginManifestVersion(fullPath, newVersion); err != nil {
			return fmt.Errorf("%s: %w", relativePath, err)
		}
	}
	if !found {
		return pluginManifestNotFoundError(pluginRoot, manifestRelativePaths)
	}
	return nil
}

// orderedField holds one top-level JSON member, preserving its original position.
type orderedField struct {
	Key   string
	Value json.RawMessage
}

// orderedObject marshals as a JSON object using field order as-is, instead of the
// alphabetical order encoding/json imposes on Go maps.
type orderedObject []orderedField

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for fieldIndex, field := range o {
		if fieldIndex > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(field.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(field.Value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// decodeOrderedTopLevel parses a top-level JSON object into orderedObject, preserving
// the on-disk member order (json.Decoder reads object keys in document order).
func decodeOrderedTopLevel(data []byte) (orderedObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := expectObjectStart(decoder); err != nil {
		return nil, err
	}
	fields, err := decodeObjectFields(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != nil { // closing '}'
		return nil, err
	}
	return fields, nil
}

// expectObjectStart consumes the opening '{' token, erroring if data isn't a JSON object.
func expectObjectStart(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected a top-level JSON object")
	}
	return nil
}

// decodeObjectFields reads key/raw-value pairs from decoder until the object closes, preserving
// their on-disk order.
func decodeObjectFields(decoder *json.Decoder) (orderedObject, error) {
	var fields orderedObject
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key in JSON object")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		fields = append(fields, orderedField{Key: key, Value: raw})
	}
	return fields, nil
}

// writePluginManifestVersion rewrites only the top-level "version" value in path, leaving
// every other field and its original order untouched.
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
	fields, err := decodeOrderedTopLevel(data)
	if err != nil {
		return err
	}
	versionJSON, err := json.Marshal(newVersion)
	if err != nil {
		return err
	}
	found := false
	for fieldIndex := range fields {
		if fields[fieldIndex].Key == manifestVersionField {
			fields[fieldIndex].Value = versionJSON
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s declares version %q but has no %q field", manifestFileName, meta.Version, manifestVersionField)
	}

	updated, err := json.MarshalIndent(fields, "", manifestJSONIndent)
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	// #nosec G306,G703 -- path is pluginRoot + knownManifestRelPaths allowlist; user-owned manifest.
	return os.WriteFile(path, updated, agentcommon.PrivateFileMode)
}
