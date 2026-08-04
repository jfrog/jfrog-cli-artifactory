package permissions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertOwnerOnly checks the 0600 bits on Unix; on Windows os.Chmod cannot express
// an owner-only DACL, so the mode is not enforced and the check is skipped.
func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	if coreutils.IsWindows() {
		t.Skip("permission bits are not enforced on Windows")
	}
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// WriteFileOwnerOnly must create a credential file at 0600, and must also tighten
// one that already exists at 0644 (os.WriteFile only applies its mode on create).
func TestWriteFileOwnerOnly(t *testing.T) {
	t.Run("new file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "creds")
		require.NoError(t, WriteFileOwnerOnly(path, []byte("secret")))
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "secret", string(content))
		assertOwnerOnly(t, path)
	})
	t.Run("pre-existing world-readable file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "creds")
		require.NoError(t, os.WriteFile(path, []byte("stale"), 0644))
		require.NoError(t, os.Chmod(path, 0644))
		require.NoError(t, WriteFileOwnerOnly(path, []byte("secret")))
		assertOwnerOnly(t, path)
	})
}

// A write into a directory that does not exist surfaces the error:
// WriteFileOwnerOnly does not create parent dirs, and our own write is not
// best-effort.
func TestWriteFileOwnerOnly_WriteError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "creds")
	assert.Error(t, WriteFileOwnerOnly(path, []byte("secret")))
}

// RestrictExisting tightens a world-readable credential file to 0600 in place.
func TestRestrictExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0644))
	require.NoError(t, os.Chmod(path, 0644))

	RestrictExisting(path)

	assertOwnerOnly(t, path)
}

// RestrictExisting is best-effort: a missing file is warned about, never fatal,
// so a resolution miss cannot fail an otherwise-successful setup.
func TestRestrictExisting_MissingFileDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		RestrictExisting(filepath.Join(t.TempDir(), "does-not-exist"))
	})
}
