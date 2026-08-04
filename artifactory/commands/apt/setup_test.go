package apt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestWriteSourcesListIdempotent_RewritesOnNarrowerSubstringLine(t *testing.T) {
	// A narrower new line that is a substring of the existing broader line must
	// still trigger a rewrite (regression: substring match left stale config).
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	require.NoError(t, os.WriteFile(path, []byte("deb https://host/repo noble main contrib\n"), 0600))

	cmd := &AptSetupCommand{}
	wrote, err := cmd.writeSourcesListIdempotent(path, "deb https://host/repo noble main")
	require.NoError(t, err)
	assert.True(t, wrote, "narrower line is not an exact match — must rewrite")

	content, _ := os.ReadFile(path)
	assert.Equal(t, "deb https://host/repo noble main\n", string(content))
}

func TestWriteSourcesListIdempotent_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	cmd := &AptSetupCommand{}

	_, err := cmd.writeSourcesListIdempotent(path, "deb https://host/repo noble main")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "sources.list must not be world-readable (contains credentials)")
}

// ── existingKeyringPath (keyring reuse across re-runs) ────────────────────────

func TestExistingKeyringPath_ReturnsPathWhenPresent(t *testing.T) {
	dir := t.TempDir()
	orig := keyringsDir
	keyringsDir = dir
	defer func() { keyringsDir = orig }()

	keyFile := filepath.Join(dir, "jfrog-myrepo-noble.asc")
	require.NoError(t, os.WriteFile(keyFile, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----"), 0644))

	assert.Equal(t, keyFile, existingKeyringPath("myrepo", "noble"))
}

func TestExistingKeyringPath_EmptyWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	orig := keyringsDir
	keyringsDir = dir
	defer func() { keyringsDir = orig }()

	assert.Equal(t, "", existingKeyringPath("myrepo", "noble"))
}

func TestExistingKeyringPath_ScopedToRepoAndDist(t *testing.T) {
	dir := t.TempDir()
	orig := keyringsDir
	keyringsDir = dir
	defer func() { keyringsDir = orig }()

	// A key for a different dist must not be treated as this dist's key.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jfrog-myrepo-jammy.asc"), []byte("x"), 0644))
	assert.Equal(t, "", existingKeyringPath("myrepo", "noble"))
}

// ── sourceHasSignedBy (downgrade detection) ───────────────────────────────────

func TestSourceHasSignedBy_TrueWhenPinned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jfrog-myrepo-noble.list")
	line := "deb [signed-by=/etc/apt/keyrings/jfrog-myrepo-noble.asc] https://host/repo noble main"
	require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0600))

	assert.True(t, sourceHasSignedBy(path))
}

func TestSourceHasSignedBy_FalseWhenBareLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jfrog-myrepo-noble.list")
	require.NoError(t, os.WriteFile(path, []byte("deb https://host/repo noble main\n"), 0600))

	assert.False(t, sourceHasSignedBy(path))
}

func TestSourceHasSignedBy_FalseWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, sourceHasSignedBy(filepath.Join(dir, "does-not-exist.list")))
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
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial does not work on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root — permission checks don't apply")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	defer func() { _ = os.Chmod(dir, 0755) }()

	err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0600)
	require.Error(t, err)

	wrapped := wrapPermErr(err)
	assert.Contains(t, wrapped.Error(), "sudo")
}

func TestWrapPermErr_WrappedPermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial does not work on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root — permission checks don't apply")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	defer func() { _ = os.Chmod(dir, 0755) }()

	innerErr := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0600)
	require.Error(t, innerErr)

	// wrapPermErr uses errors.Is, which walks the whole chain — nested/multiply
	// wrapped permission errors must still get the sudo hint.
	doubleWrapped := fmt.Errorf("import GPG key: %w", fmt.Errorf("write public key: %w", innerErr))
	result := wrapPermErr(doubleWrapped)
	assert.Contains(t, result.Error(), "sudo")
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

func TestRunRemove_GlobMetacharDistMatchesNothing(t *testing.T) {
	// A --dist containing a glob metacharacter must not be expanded as a pattern:
	// removal filters by literal suffix, so "*" matches no real jfrog-<repo>-<dist>
	// file and leaves every other dist's config untouched.
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

	cmd := &AptSetupCommand{dist: "*"}
	require.NoError(t, cmd.runRemove())

	// Nothing removed — "*" is treated literally, not as a glob.
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoA-noble.list"))
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoB-jammy.list"))
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoA-noble.pref"))
}

func TestRunRemove_RepoScoped(t *testing.T) {
	// --remove --repo=A must delete only repo A's files, leaving other repos intact.
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
		"jfrog-repoA-noble.list", "jfrog-repoA-noble.pref", "jfrog-repoA-jammy.list",
		"jfrog-repoB-noble.list", "jfrog-repoB-noble.pref",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600))
	}

	cmd := &AptSetupCommand{repoName: "repoA"}
	require.NoError(t, cmd.runRemove())

	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-noble.list"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-noble.pref"))
	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-jammy.list"))
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoB-noble.list"), "other repo must survive")
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoB-noble.pref"), "other repo must survive")
}

func TestRunRemove_RepoAndDistScoped(t *testing.T) {
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
		"jfrog-repoA-noble.list", "jfrog-repoA-jammy.list", "jfrog-repoB-noble.list",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600))
	}

	cmd := &AptSetupCommand{repoName: "repoA", dist: "noble"}
	require.NoError(t, cmd.runRemove())

	assert.NoFileExists(t, filepath.Join(dir, "jfrog-repoA-noble.list"))
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoA-jammy.list"), "repoA other dist must survive")
	assert.FileExists(t, filepath.Join(dir, "jfrog-repoB-noble.list"), "other repo must survive")
}

func TestWriteSourcesListIdempotent_TightensExistingLoosePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.list")
	// Pre-existing file with loose (world-readable) perms.
	require.NoError(t, os.WriteFile(path, []byte("deb https://old-host/repo noble main\n"), 0644))

	cmd := &AptSetupCommand{}
	wrote, err := cmd.writeSourcesListIdempotent(path, "deb https://new-host/repo noble main")
	require.NoError(t, err)
	assert.True(t, wrote)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "credential-bearing file must be tightened to 0600")
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
