package common

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

	zipPath, tmpDir, hash, err := ZipPublishBundle(ZipPublishOptions{
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
	defer func() { _ = os.RemoveAll(tmpDir) }()
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

func TestShouldExcludePublishPath(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, dir, ".git")
	mustMkdir(t, dir, "node_modules")
	mustWritePublishTestFile(t, dir, "cache.pyc", "x")
	mustWritePublishTestFile(t, dir, "src/main.go", "ok")

	tests := []struct {
		relPath string
		want    bool
	}{
		{relPath: ".", want: false},
		{relPath: ".git", want: true},
		{relPath: "node_modules", want: true},
		{relPath: "cache.pyc", want: true},
		{relPath: filepath.Join("src", "main.go"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(dir, tt.relPath))
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if got := ShouldExcludePublishPath(tt.relPath, info); got != tt.want {
				t.Fatalf("ShouldExcludePublishPath(%q) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

func mustMkdir(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(full, DefaultDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func TestComputeSHA256RejectsNonLocalRelativePath(t *testing.T) {
	_, err := ComputeSHA256("../outside.zip")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestComputeSHA256AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.zip")
	if err := os.WriteFile(path, []byte("payload"), PrivateFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
	hash, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

func mustWritePublishTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), DefaultDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), PrivateFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
}
