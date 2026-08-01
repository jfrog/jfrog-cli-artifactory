package permissions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ChmodOwnerOnly must tighten a world-readable credential file to 0600, including
// one that already exists (WriteFile only applies its mode when it creates a file).
func TestChmodOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0644))

	require.NoError(t, ChmodOwnerOnly(path))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if coreutils.IsWindows() {
		// os.Chmod cannot express an owner-only DACL on Windows, so the file keeps
		// the ACLs it inherited; ChmodOwnerOnly is a documented no-op there.
		t.Skip("permission bits are not enforced on Windows")
	}
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// On Unix a missing file surfaces the chmod error; on Windows the call is a no-op
// and returns nil regardless of the path.
func TestChmodOwnerOnly_MissingFile(t *testing.T) {
	err := ChmodOwnerOnly(filepath.Join(t.TempDir(), "does-not-exist"))
	if coreutils.IsWindows() {
		assert.NoError(t, err)
		return
	}
	assert.Error(t, err)
}
