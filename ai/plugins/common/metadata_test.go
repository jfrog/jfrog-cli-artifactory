package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aicommon "github.com/jfrog/jfrog-cli-artifactory/ai/common"
)

func writePluginJSON(t *testing.T, root, rel string, meta map[string]string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), aicommon.DefaultDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fullPath, data, aicommon.PrivateFileMode); err != nil {
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

func TestValidateAndResolvePluginMeta_FirstManifestWins(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "root", "version": "1.0.0"})
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "cursor", "version": "9.9.9"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "root" || meta.Version != "1.0.0" {
		t.Fatalf("expected root plugin.json to win, got %+v", meta)
	}
}

func TestValidateAndResolvePluginMeta_MissingAll(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateAndResolvePluginMeta(dir, "")
	if err == nil || !strings.Contains(err.Error(), "no plugin.json") {
		t.Fatalf("expected no-manifest error, got %v", err)
	}
}

func TestValidateAndResolvePluginMeta_DefaultVersion(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Version != DefaultPluginVersion {
		t.Fatalf("expected default %s, got %s", DefaultPluginVersion, meta.Version)
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

func TestDiscoverPluginManifests_OnlyKnownPaths(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, ".github/plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	// An unknown location must not be discovered.
	writePluginJSON(t, dir, "other-dir/plugin.json", map[string]string{"name": "ignore-me", "version": "9.9.9"})

	manifests, err := DiscoverPluginManifests(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected exactly one manifest, got %d: %+v", len(manifests), manifests)
	}
	if _, ok := manifests[".github/plugin/plugin.json"]; !ok {
		t.Fatalf("expected .github/plugin/plugin.json in manifests, got %+v", manifests)
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

func TestUpdatePluginManifestVersions_BeforePublishOrder(t *testing.T) {
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
		t.Fatalf("version on disk = %q, want 1.0.2 before zip/publish", doc["version"])
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
	if err := os.WriteFile(path, []byte(raw), aicommon.PrivateFileMode); err != nil {
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
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	if err := UpdatePluginManifestVersions(dir, "2.0.0"); err != nil {
		t.Fatalf("update: %v", err)
	}
	root, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	if !strings.Contains(string(root), "2.0.0") {
		t.Fatalf("expected root manifest updated, got %s", string(root))
	}
	cursor, err := os.ReadFile(filepath.Join(dir, ".cursor-plugin/plugin.json"))
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if strings.Contains(string(cursor), "2.0.0") {
		t.Fatalf("expected secondary manifest unchanged, got %s", string(cursor))
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
