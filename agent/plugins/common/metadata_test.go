package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
)

func writePluginJSON(t *testing.T, root, rel string, meta map[string]string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), agentcommon.DefaultDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fullPath, data, agentcommon.PrivateFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestValidateAndResolvePluginMeta_SingleRootManifest(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "2.4.1"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "demo" || meta.Version != "2.4.1" {
		t.Fatalf("unexpected meta %+v", meta)
	}
}

func TestValidateAndResolvePluginMeta_ClaudeManifestWins(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "root", "version": "1.0.0"})
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "cursor", "version": "8.8.8"})
	writePluginJSON(t, dir, ".claude-plugin/plugin.json", map[string]string{"name": "claude", "version": "9.9.9"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "claude" || meta.Version != "9.9.9" {
		t.Fatalf("expected .claude-plugin/plugin.json to win, got %+v", meta)
	}
}

func TestLoadPluginManifestPaths_MergesConfigBeforeDefaults(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{
		"plugin-manifest-paths": ["custom-dir/plugin.json", "plugin.json"]
	}`)

	got, err := loadPluginManifestPaths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPrefix := []string{
		"custom-dir/plugin.json",
		"plugin.json",
		".claude-plugin/plugin.json",
		".cursor-plugin/plugin.json",
	}
	if len(got) < len(wantPrefix) {
		t.Fatalf("unexpected paths %v", got)
	}
	for i, want := range wantPrefix {
		if got[i] != want {
			t.Fatalf("got[%d] = %q, want %q (full list %v)", i, got[i], want, got)
		}
	}
	if len(got) != len(knownManifestRelPaths)+1 {
		t.Fatalf("expected %d paths (config + defaults minus duplicate plugin.json), got %d: %v",
			len(knownManifestRelPaths)+1, len(got), got)
	}
}

func TestLoadPluginManifestPaths_ConfigTakesPriorityOverDefault(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{"plugin-manifest-paths": ["custom-dir/plugin.json"]}`)

	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "root", "version": "1.0.0"})
	writePluginJSON(t, dir, "custom-dir/plugin.json", map[string]string{"name": "custom", "version": "2.0.0"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "custom" {
		t.Fatalf("expected custom-dir manifest, got %+v", meta)
	}
}

func TestLoadPluginManifestPaths_FallbackOrder(t *testing.T) {
	testutil.WithJfrogHome(t)

	got, err := loadPluginManifestPaths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0] != ".claude-plugin/plugin.json" {
		t.Fatalf("expected .claude-plugin first, got %v", got)
	}
}

func TestKnownManifestRelPaths_AgentPriorityOrder(t *testing.T) {
	want := []string{
		".claude-plugin/plugin.json",
		".cursor-plugin/plugin.json",
		".codex-plugin/plugin.json",
		"plugin.json",
		".github/plugin/plugin.json",
		".plugin/plugin.json",
	}
	if len(knownManifestRelPaths) != len(want) {
		t.Fatalf("knownManifestRelPaths length = %d, want %d", len(knownManifestRelPaths), len(want))
	}
	for i, p := range want {
		if knownManifestRelPaths[i] != p {
			t.Fatalf("knownManifestRelPaths[%d] = %q, want %q", i, knownManifestRelPaths[i], p)
		}
	}
}

func TestValidateAndResolvePluginMeta_MissingAll(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateAndResolvePluginMeta(dir, "")
	if err == nil {
		t.Fatal("expected no-manifest error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "no plugin.json") {
		t.Fatalf("expected no-manifest error, got %v", err)
	}
	if !strings.Contains(errMsg, "plugin-manifest-paths") {
		t.Fatalf("expected agent-config hint, got %v", err)
	}
	if !strings.Contains(errMsg, "agent-config.json") {
		t.Fatalf("expected agent-config.json path in hint, got %v", err)
	}
}

func TestValidateAndResolvePluginMeta_DefaultVersion(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Version != defaultPluginVersion {
		t.Fatalf("expected default %s, got %s", defaultPluginVersion, meta.Version)
	}
}

func TestValidateAndResolvePluginMeta_VersionFlagOverridesConsensus(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	meta, err := ValidateAndResolvePluginMeta(dir, "3.2.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Version != "3.2.1" {
		t.Fatalf("expected flag override 3.2.1, got %s", meta.Version)
	}
}

func TestValidateAndResolvePluginMeta_VersionFlagOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo"})

	meta, err := ValidateAndResolvePluginMeta(dir, "0.0.7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Version != "0.0.7" {
		t.Fatalf("expected flag override 0.0.7, got %s", meta.Version)
	}
}

func TestValidateAndResolvePluginMeta_EmptyName(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "", "version": "1.0.0"})

	_, err := ValidateAndResolvePluginMeta(dir, "")
	if err == nil || !strings.Contains(err.Error(), "missing required 'name'") {
		t.Fatalf("expected missing-name error, got %v", err)
	}
}

func TestFindPrimaryPluginManifest_OnlyKnownPaths(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, ".github/plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	// An unknown location must not be discovered.
	writePluginJSON(t, dir, "other-dir/plugin.json", map[string]string{"name": "ignore-me", "version": "9.9.9"})

	relPath, meta, err := findPrimaryPluginManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if relPath != ".github/plugin/plugin.json" {
		t.Fatalf("expected .github/plugin/plugin.json, got %s", relPath)
	}
	if meta.Name != "demo" || meta.Version != "1.0.0" {
		t.Fatalf("unexpected meta %+v", meta)
	}
}

func TestValidateSlug(t *testing.T) {
	good := []string{"foo", "foo-bar", "foo123", "a", "4chan-reader"}
	for _, s := range good {
		if err := ValidateSlug(s); err != nil {
			t.Fatalf("slug %q should be valid: %v", s, err)
		}
	}
	bad := []string{"", "-foo", "Foo", "foo bar", "foo/bar"}
	for _, s := range bad {
		if err := ValidateSlug(s); err == nil {
			t.Fatalf("slug %q should be invalid", s)
		}
	}
}

func TestUpdatePluginManifestVersions_UpdatesPrimaryManifest(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	meta, err := ValidateAndResolvePluginMeta(dir, "1.0.2")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if meta.ManifestVersion != "1.0.0" || meta.Version != "1.0.2" {
		t.Fatalf("unexpected meta %+v", meta)
	}
	if meta.ManifestVersion == meta.Version {
		t.Fatal("expected manifest and resolved versions to differ for --version override")
	}

	if err := UpdatePluginManifestVersions(dir, "1.0.2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]string
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["version"] != "1.0.2" {
		t.Fatalf("version on disk = %q, want 1.0.2", doc["version"])
	}
}

func TestWritePluginManifestVersion_IndentedJSON(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	if err := writePluginManifestVersion(filepath.Join(dir, "plugin.json"), "1.0.2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "\n") {
		t.Fatalf("expected indented json with newlines, got %s", body)
	}
	if !strings.Contains(body, `"version": "1.0.2"`) {
		t.Fatalf("expected updated version in indented json, got %s", body)
	}
}

func TestWritePluginManifestVersion_ReplacesOnlyFirstVersionField(t *testing.T) {
	raw := `{
  "version": "1.0.0",
  "nested": { "version": "9.9.9" }
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, []byte(raw), agentcommon.PrivateFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writePluginManifestVersion(path, "1.0.2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "    \"version\": \"1.0.2\"") {
		t.Fatalf("expected top-level version updated with indent, got %s", body)
	}
	if !strings.Contains(body, `"version": "9.9.9"`) {
		t.Fatalf("expected nested version unchanged, got %s", body)
	}
}

func TestUpdatePluginManifestVersions_OnlyPrimaryManifest(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, ".claude-plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	if err := UpdatePluginManifestVersions(dir, "2.0.0"); err != nil {
		t.Fatalf("update: %v", err)
	}
	primary, err := os.ReadFile(filepath.Join(dir, ".claude-plugin/plugin.json"))
	if err != nil {
		t.Fatalf("read primary: %v", err)
	}
	if !strings.Contains(string(primary), "2.0.0") {
		t.Fatalf("expected .claude-plugin/plugin.json updated, got %s", string(primary))
	}
	root, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	if strings.Contains(string(root), "2.0.0") {
		t.Fatalf("expected secondary manifest unchanged, got %s", string(root))
	}
}

func TestWritePluginManifestVersion_PreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{
		"name":    "autoagent2",
		"version": "1.0.0",
		"author":  "Author Frog",
	})

	path := filepath.Join(dir, "plugin.json")
	if err := writePluginManifestVersion(path, "1.0.2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	var doc map[string]string
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["version"] != "1.0.2" {
		t.Fatalf("version = %q, want 1.0.2", doc["version"])
	}
	if doc["name"] != "autoagent2" || doc["author"] != "Author Frog" {
		t.Fatalf("other fields changed: %+v", doc)
	}
}

func TestUpdatePluginManifestVersions_SkipsWhenNoManifestVersion(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo"})

	meta, err := ValidateAndResolvePluginMeta(dir, "2.0.0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if meta.ManifestVersion != "" {
		t.Fatalf("ManifestVersion = %q, want empty", meta.ManifestVersion)
	}

	if err := UpdatePluginManifestVersions(dir, "2.0.0"); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), `"version"`) {
		t.Fatalf("expected no version field inserted, got %s", string(data))
	}
}

func TestValidateVersion(t *testing.T) {
	good := []string{"1.0.0", "1.2.3-rc.1", "0.1.0+build.1"}
	for _, v := range good {
		if err := ValidateVersion(v); err != nil {
			t.Fatalf("version %q should be valid: %v", v, err)
		}
	}
	bad := []string{"", "..", "1.0/.0", "not-a-version", "1.0..0"}
	for _, v := range bad {
		if err := ValidateVersion(v); err == nil {
			t.Fatalf("version %q should be invalid", v)
		}
	}
}
