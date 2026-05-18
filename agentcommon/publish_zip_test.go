package agentcommon

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipPublishBundleSkipsExcluded(t *testing.T) {
	dir := t.TempDir()
	mustWritePublishTestFile(t, dir, "README.md", "hi")
	mustWritePublishTestFile(t, dir, ".git/HEAD", "junk")
	mustWritePublishTestFile(t, dir, "__pycache__/x.pyc", "junk")
	mustWritePublishTestFile(t, dir, "src/main.go", "package main")

	zipPath, hash, err := ZipPublishBundle(ZipPublishOptions{
		SourceDir:      dir,
		Slug:           "demo",
		Version:        "1.0.0",
		TempDirPrefix:  "agent-plugin-publish-",
		ContentLabel:   "plugin",
		HashWhileWrite: true,
	})
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

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

func mustWritePublishTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
