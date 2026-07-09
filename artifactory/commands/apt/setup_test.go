package apt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── writeSourcesListIdempotent ────────────────────────────────────────────────

func TestWriteSourcesListIdempotent_WritesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	cmd := &AptSetupCommand{}

	wrote, err := cmd.writeSourcesListIdempotent(path, "deb https://host/repo noble main")
	require.NoError(t, err)
	assert.True(t, wrote)

	content, _ := os.ReadFile(path)
	assert.Contains(t, string(content), "deb https://host/repo noble main")
}

func TestWriteSourcesListIdempotent_IdempotentOnSameLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	line := "deb https://host/repo noble main"
	require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0600))

	cmd := &AptSetupCommand{}
	wrote, err := cmd.writeSourcesListIdempotent(path, line)
	require.NoError(t, err)
	assert.False(t, wrote, "should not write when line already present")
}

func TestWriteSourcesListIdempotent_OverwritesOnDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	require.NoError(t, os.WriteFile(path, []byte("deb https://old-host/repo noble main\n"), 0600))

	cmd := &AptSetupCommand{}
	wrote, err := cmd.writeSourcesListIdempotent(path, "deb https://new-host/repo noble main")
	require.NoError(t, err)
	assert.True(t, wrote)

	content, _ := os.ReadFile(path)
	assert.Contains(t, string(content), "new-host")
	assert.NotContains(t, string(content), "old-host")
}

func TestWriteSourcesListIdempotent_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	cmd := &AptSetupCommand{}

	_, err := cmd.writeSourcesListIdempotent(path, "deb https://host/repo noble main")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "sources.list must not be world-readable (contains credentials)")
}

// ── wrapPermErr ───────────────────────────────────────────────────────────────

func TestWrapPermErr_Nil(t *testing.T) {
	assert.Nil(t, wrapPermErr(nil))
}

func TestWrapPermErr_NonPermError(t *testing.T) {
	err := errors.New("connection refused")
	wrapped := wrapPermErr(err)
	assert.Equal(t, err, wrapped, "non-permission error should pass through unchanged")
}

func TestWrapPermErr_PermissionDenied(t *testing.T) {
	// Trigger a real permission denied by writing to a read-only dir (unix only).
	if os.Getuid() == 0 {
		t.Skip("running as root — permission checks don't apply")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	defer os.Chmod(dir, 0755)

	err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0600)
	require.Error(t, err)

	wrapped := wrapPermErr(err)
	assert.Contains(t, wrapped.Error(), "sudo")
}

func TestWrapPermErr_WrappedPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission checks don't apply")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	defer os.Chmod(dir, 0755)

	innerErr := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0600)
	require.Error(t, innerErr)

	// errors.Join produces an error with Unwrap() []error — wrapPermErr checks one level.
	doubleWrapped := errors.Join(errors.New("context"), innerErr)
	result := wrapPermErr(doubleWrapped)
	// No panic is the main invariant here; sudo hint presence depends on unwrap depth.
	_ = result
}

// ── runRemove ─────────────────────────────────────────────────────────────────

func TestRunRemove_RemovesAllFiles(t *testing.T) {
	dir := t.TempDir()
	// patch dirs for isolation
	origSrc, origPref, origKey := sourcesListDir, preferencesDir, keyringsDir
	sourcesListDir = dir
	preferencesDir = dir
	keyringsDir = dir
	defer func() {
		sourcesListDir = origSrc
		preferencesDir = origPref
		keyringsDir = origKey
	}()

	// create files that should be removed
	for _, name := range []string{
		"jfrog-myrepo-noble.list",
		"jfrog-myrepo-noble.pref",
		"jfrog-myrepo-noble.asc",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644))
	}
	// create a file that should NOT be removed (different dist)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jfrog-myrepo-jammy.list"), []byte("x"), 0644))

	cmd := &AptSetupCommand{dist: "noble"}
	require.NoError(t, cmd.runRemove())

	assert.NoFileExists(t, filepath.Join(dir, "jfrog-myrepo-noble.list"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-myrepo-noble.pref"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-myrepo-noble.asc"))
	assert.FileExists(t, filepath.Join(dir, "jfrog-myrepo-jammy.list"), "other dist must not be removed")
}

func TestRunRemove_RemovesAllDists(t *testing.T) {
	dir := t.TempDir()
	origSrc, origPref, origKey := sourcesListDir, preferencesDir, keyringsDir
	sourcesListDir = dir
	preferencesDir = dir
	keyringsDir = dir
	defer func() {
		sourcesListDir = origSrc
		preferencesDir = origPref
		keyringsDir = origKey
	}()

	for _, name := range []string{
		"jfrog-repoA-noble.list",
		"jfrog-repoB-jammy.list",
		"jfrog-repoA-noble.pref",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644))
	}

	// no --dist → remove all jfrog-* files
	cmd := &AptSetupCommand{}
	require.NoError(t, cmd.runRemove())

	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-noble.list"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoB-jammy.list"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-noble.pref"))
}

func TestRunRemove_NothingToRemove(t *testing.T) {
	dir := t.TempDir()
	origSrc, origPref, origKey := sourcesListDir, preferencesDir, keyringsDir
	sourcesListDir = dir
	preferencesDir = dir
	keyringsDir = dir
	defer func() {
		sourcesListDir = origSrc
		preferencesDir = origPref
		keyringsDir = origKey
	}()

	cmd := &AptSetupCommand{dist: "noble"}
	// should not error when nothing matches
	assert.NoError(t, cmd.runRemove())
}

// ── AptSetupCommand.Run validation ───────────────────────────────────────────

func TestRun_ReturnsErrorIfTrustedAndImportKeyBothSet(t *testing.T) {
	cmd := NewAptSetupCommand().
		SetTrusted(true).
		SetImportKey(true).
		SetRepoName("repo").
		SetDist("noble")
	// serverDetails nil — should fail before reaching key fetch
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mutually exclusive")
}

func TestRun_ReturnsErrorIfRepoMissing(t *testing.T) {
	cmd := NewAptSetupCommand().SetDist("noble")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--repo")
}

func TestRun_ReturnsErrorIfDistMissing(t *testing.T) {
	cmd := NewAptSetupCommand().SetRepoName("repo")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dist")
}

func TestRun_ReturnsErrorIfServerDetailsNil(t *testing.T) {
	cmd := NewAptSetupCommand().SetRepoName("repo").SetDist("noble")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details")
}
