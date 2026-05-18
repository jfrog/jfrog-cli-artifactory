package publish

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveZipUsesPrebuiltWhenPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"demo","version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
	zipDir := filepath.Join(dir, "zip")
	if err := os.MkdirAll(zipDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	prebuiltPath := filepath.Join(zipDir, "demo_1.0.0.zip")
	if err := os.WriteFile(prebuiltPath, []byte("prebuilt-content"), 0o600); err != nil {
		t.Fatalf("write prebuilt: %v", err)
	}

	pc := NewPublishCommand().SetPluginDir(dir)
	gotPath, gotHash, prebuilt, err := pc.resolveZip("demo", "1.0.0")
	if err != nil {
		t.Fatalf("resolveZip: %v", err)
	}
	if !prebuilt {
		t.Fatalf("expected prebuilt=true")
	}
	if gotPath != filepath.Clean(prebuiltPath) {
		t.Fatalf("expected prebuilt path %q, got %q", prebuiltPath, gotPath)
	}
	if gotHash != "" {
		t.Fatalf("prebuilt path should return empty hash (caller hashes on disk); got %q", gotHash)
	}
}

func TestResolveZipBuildsWhenNoPrebuilt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"demo","version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}

	pc := NewPublishCommand().SetPluginDir(dir)
	gotPath, gotHash, prebuilt, err := pc.resolveZip("demo", "1.0.0")
	if err != nil {
		t.Fatalf("resolveZip: %v", err)
	}
	defer func() { _ = os.Remove(gotPath) }()

	if prebuilt {
		t.Fatalf("expected prebuilt=false when no prebuilt zip exists")
	}
	if !strings.HasSuffix(gotPath, "demo-1.0.0.zip") {
		t.Fatalf("expected zip name suffix 'demo-1.0.0.zip', got %q", gotPath)
	}
	if gotHash == "" {
		t.Fatalf("expected non-empty sha256 hex from streaming hasher")
	}

	// Verify the hash matches a fresh on-disk computation.
	want, err := computeSHA256(gotPath)
	if err != nil {
		t.Fatalf("computeSHA256: %v", err)
	}
	if gotHash != want {
		t.Fatalf("streaming hash %q != on-disk hash %q", gotHash, want)
	}
}

func TestResolveZipRejectsTraversalVersion(t *testing.T) {
	pc := NewPublishCommand().SetPluginDir(t.TempDir())
	_, _, _, err := pc.resolveZip("demo", "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path-traversal version")
	}
}

func TestZipPluginFolderSkipsExcluded(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite("README.md", "hi")
	mustWrite(".git/HEAD", "junk")
	mustWrite("__pycache__/x.pyc", "junk")
	mustWrite("src/main.go", "package main")

	zipPath, hash, err := zipPluginFolder(dir, "demo", "1.0.0")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify the hash equals the on-disk computation.
	f, err := os.Open(zipPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != hash {
		t.Fatal("returned hash does not match disk")
	}

	// Verify excluded files are absent.
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer func() { _ = zr.Close() }()
	names := make(map[string]bool, len(zr.File))
	for _, file := range zr.File {
		names[file.Name] = true
	}
	if !names["README.md"] || !names[filepath.Join("src", "main.go")] {
		t.Fatalf("expected included files, got %v", names)
	}
	for name := range names {
		if strings.HasPrefix(name, ".git") || strings.HasPrefix(name, "__pycache__") {
			t.Fatalf("excluded path was included: %s", name)
		}
	}
}
