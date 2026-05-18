package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePluginJSON(t *testing.T, root, rel string, meta map[string]string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
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

func TestValidateAndResolvePluginMeta_ConsistentMultiManifest(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	writePluginJSON(t, dir, ".claude-plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

	meta, err := ValidateAndResolvePluginMeta(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "demo" || meta.Version != "1.0.0" {
		t.Fatalf("unexpected meta %+v", meta)
	}
}

func TestValidateAndResolvePluginMeta_ConflictingNames(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "foo", "version": "1.0.0"})
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "bar", "version": "1.0.0"})

	_, err := ValidateAndResolvePluginMeta(dir, "")
	if err == nil || !strings.Contains(err.Error(), "name mismatch") {
		t.Fatalf("expected name mismatch error, got %v", err)
	}
}

func TestValidateAndResolvePluginMeta_ConflictingVersions(t *testing.T) {
	dir := t.TempDir()
	writePluginJSON(t, dir, "plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})
	writePluginJSON(t, dir, ".claude-plugin/plugin.json", map[string]string{"name": "demo", "version": "2.0.0"})

	_, err := ValidateAndResolvePluginMeta(dir, "")
	if err == nil || !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("expected version mismatch error, got %v", err)
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
	writePluginJSON(t, dir, ".cursor-plugin/plugin.json", map[string]string{"name": "demo", "version": "1.0.0"})

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
	good := []string{"foo", "foo-bar", "foo123", "a"}
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

func TestValidateVersion(t *testing.T) {
	good := []string{"1.0.0", "1.2.3-rc1", "0.1.0+build.1"}
	for _, v := range good {
		if err := ValidateVersion(v); err != nil {
			t.Fatalf("version %q should be valid: %v", v, err)
		}
	}
	bad := []string{"", "..", "1.0/.0", "1.0\\0", "1.0..0"}
	for _, v := range bad {
		if err := ValidateVersion(v); err == nil {
			t.Fatalf("version %q should be invalid", v)
		}
	}
}
