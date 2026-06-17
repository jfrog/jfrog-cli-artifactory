package healcomponents

import (
	"os"
	"path/filepath"
	"testing"

	testsutils "github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyLockfiles_WritesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("a"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app/gradle.lockfile"), []byte("b"), 0644))

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "package-lock.json", Content: []byte("a-healed")},
		{Path: "app/gradle.lockfile", Content: []byte("b-healed")},
	}, nil)
	require.NoError(t, err)
	defer testsutils.RemoveAllAndAssert(t, dir)

	a, _ := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	b, _ := os.ReadFile(filepath.Join(dir, "app/gradle.lockfile"))
	assert.Equal(t, "a-healed", string(a))
	assert.Equal(t, "b-healed", string(b))

	require.NoError(t, restore())
}
