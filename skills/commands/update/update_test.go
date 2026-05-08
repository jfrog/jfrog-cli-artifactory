package update

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── readInstalledVersion ─────────────────────────────────────────────────────

func TestReadInstalledVersion_Found(t *testing.T) {
	dir := skillDir(t, "web-search", `---
name: web-search
version: 1.2.3
---
`)
	version, err := readInstalledVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", version)
}

func TestReadInstalledVersion_NoVersionField(t *testing.T) {
	dir := skillDir(t, "web-search", `---
name: web-search
description: No version here
---
`)
	version, err := readInstalledVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "", version, "empty version when SKILL.md has no version field")
}

func TestReadInstalledVersion_NotInstalled(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "nonexistent-skill")

	_, err := readInstalledVersion(missing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed at")
	assert.Contains(t, err.Error(), "jf skills install")
}

func TestRunUpdate_PathDoesNotExist(t *testing.T) {
	err := validateInstallBase("/nonexistent/path/xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid directory")
}

func TestValidateInstallBase_NotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0644))

	err := validateInstallBase(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid directory")
}

func TestReadInstalledVersion_InvalidFrontmatter(t *testing.T) {
	dir := skillDir(t, "bad-skill", "# No frontmatter at all\n")
	_, err := readInstalledVersion(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read installed skill metadata")
}

// ── selectVersion ────────────────────────────────────────────────────────────

func TestSelectVersion_ExactMatchReturnsIt(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := selectVersion(available, "1.1.0", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got)
}

func TestSelectVersion_LatestEmpty(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := selectVersion(available, "", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got)
}

func TestSelectVersion_LatestKeyword(t *testing.T) {
	available := []string{"1.0.0", "3.0.0", "2.0.0"}
	got, err := selectVersion(available, "latest", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", got)
}

func TestSelectVersion_NotFoundQuiet(t *testing.T) {
	available := []string{"1.0.0", "1.1.0"}
	_, err := selectVersion(available, "9.9.9", "skills-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectVersion_EmptyAvailableList(t *testing.T) {
	_, err := selectVersion([]string{}, "", "skills-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest version")
}

// ── reserveUpdateBackupPath ──────────────────────────────────────────────────

func TestReserveUpdateBackupPath(t *testing.T) {
	base := t.TempDir()
	p, err := reserveUpdateBackupPath(base, "skill-a")
	require.NoError(t, err)
	assert.Contains(t, p, ".skill-a.jfrog-update-backup-")
	_, statErr := os.Stat(p)
	require.True(t, errors.Is(statErr, fs.ErrNotExist), "reserved path must not exist until rename")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func skillDir(t *testing.T, slug, skillMD string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, slug)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))
	return dir
}
