package publish

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPublishTestContext(args ...string) *components.Context {
	ctx := &components.Context{Arguments: args}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

func TestRunPublish_MissingPathArgument(t *testing.T) {
	err := RunPublish(newPublishTestContext())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage: jf agent plugins publish")
}

func TestValidatePluginDir(t *testing.T) {
	t.Run("path is not a directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "not-a-dir")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), common.PrivateFileMode))

		_, err := validatePluginDir(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid directory")
	})

	t.Run("path does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing-plugin-dir")
		_, err := validatePluginDir(missing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid directory")
	})

	t.Run("valid directory", func(t *testing.T) {
		pluginDir := t.TempDir()
		abs, err := validatePluginDir(pluginDir)
		require.NoError(t, err)
		info, err := os.Stat(abs)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func zipPluginFolder(pluginDir, slug, version string) (zipPath, tmpDir, hash string, err error) {
	return common.ZipPublishBundle(common.ZipPublishOptions{
		SourceDir:      pluginDir,
		Slug:           slug,
		Version:        version,
		TempDirPrefix:  "agent-plugin-publish-",
		ContentLabel:   "plugin",
		HashWhileWrite: true,
	})
}

func TestResolveZipUsesPrebuiltWhenPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"demo","version":"1.0.0"}`), common.PrivateFileMode); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
	zipDir := filepath.Join(dir, "zip")
	if err := os.MkdirAll(zipDir, common.DefaultDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	prebuiltPath := filepath.Join(zipDir, "demo_1.0.0.zip")
	if err := os.WriteFile(prebuiltPath, []byte("prebuilt-content"), common.PrivateFileMode); err != nil {
		t.Fatalf("write prebuilt: %v", err)
	}

	pc := NewPublishCommand().SetPluginDir(dir)
	gotPath, gotHash, _, prebuilt, err := pc.resolveZip("demo", "1.0.0")
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
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"demo","version":"1.0.0"}`), common.PrivateFileMode); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}

	pc := NewPublishCommand().SetPluginDir(dir)
	gotPath, gotHash, gotTmpDir, prebuilt, err := pc.resolveZip("demo", "1.0.0")
	if err != nil {
		t.Fatalf("resolveZip: %v", err)
	}
	defer func() { _ = os.RemoveAll(gotTmpDir) }()

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
	want, err := common.ComputeSHA256(gotPath)
	if err != nil {
		t.Fatalf("computeSHA256: %v", err)
	}
	if gotHash != want {
		t.Fatalf("streaming hash %q != on-disk hash %q", gotHash, want)
	}
}

func TestResolveZipRejectsTraversalVersion(t *testing.T) {
	pc := NewPublishCommand().SetPluginDir(t.TempDir())
	_, _, _, _, err := pc.resolveZip("demo", "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path-traversal version")
	}
}

func TestZipPluginFolderSkipsExcluded(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), common.DefaultDirMode); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), common.PrivateFileMode); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite("README.md", "hi")
	mustWrite(".git/HEAD", "junk")
	mustWrite("__pycache__/x.pyc", "junk")
	mustWrite("src/main.go", "package main")

	zipPath, tmpDir, hash, err := zipPluginFolder(dir, "demo", "1.0.0")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
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

func TestResolveVersionCollision_NonInteractiveUnknownExistence(t *testing.T) {
	orig := packageVersionExists
	defer func() { packageVersionExists = orig }()

	packageVersionExists = func(*config.ServerDetails, string, string, string) (bool, error) {
		return false, fmt.Errorf("%w: %w", common.ErrVersionExistenceUnknown, errors.New("unparseable response"))
	}

	pc := NewPublishCommand().SetQuiet(true).SetRepoKey("plugins-local")
	_, err := pc.resolveVersionCollision("demo", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not verify whether version")
}

func TestResolveVersionCollision_NonInteractiveVersionExists(t *testing.T) {
	orig := packageVersionExists
	defer func() { packageVersionExists = orig }()

	packageVersionExists = func(*config.ServerDetails, string, string, string) (bool, error) {
		return true, nil
	}

	pc := NewPublishCommand().SetQuiet(true).SetRepoKey("plugins-local")
	_, err := pc.resolveVersionCollision("demo", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestResolveVersionCollision_NonInteractiveVersionAbsent(t *testing.T) {
	orig := packageVersionExists
	defer func() { packageVersionExists = orig }()

	packageVersionExists = func(*config.ServerDetails, string, string, string) (bool, error) {
		return false, nil
	}

	pc := NewPublishCommand().SetQuiet(true).SetRepoKey("plugins-local")
	version, err := pc.resolveVersionCollision("demo", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", version)
}

func TestResolveVersionCollision_InteractiveUnknownExistenceProceeds(t *testing.T) {
	t.Setenv("CI", "")

	orig := packageVersionExists
	defer func() { packageVersionExists = orig }()

	packageVersionExists = func(*config.ServerDetails, string, string, string) (bool, error) {
		return false, fmt.Errorf("%w: %w", common.ErrVersionExistenceUnknown, errors.New("unparseable response"))
	}

	pc := NewPublishCommand().SetRepoKey("plugins-local")
	version, err := pc.resolveVersionCollision("demo", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", version)
}
