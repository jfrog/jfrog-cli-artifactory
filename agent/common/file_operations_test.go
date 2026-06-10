package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde relative path", in: "~/x/y", want: filepath.Join(home, "x/y")},
		{name: "absolute path", in: "/abs/path", want: "/abs/path"},
		{name: "tilde only unchanged", in: "~", want: "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandHome(tt.in))
		})
	}
}

func TestValidateExistingDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ValidateExistingDir(dir))

	err := ValidateExistingDir(filepath.Join(dir, "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644))

	err := CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data))

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data))
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "source.txt")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o644))

	dst := filepath.Join(t.TempDir(), "dest.txt")
	require.NoError(t, CopyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(data))
}

func TestEnsureDestinationDir_CreatesUnderExistingParent(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "install-x")
	require.NoError(t, EnsureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_CreatesNestedPath(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "nested", "plugins", "alpha")
	require.NoError(t, EnsureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_RejectsFileAtDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(dest, []byte("hi"), 0o644))
	err := EnsureDestinationDir(dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestMovePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	require.NoError(t, os.WriteFile(src, []byte("moved"), 0o644))
	dst := filepath.Join(dir, "new.txt")

	require.NoError(t, MovePath(src, dst))
	_, err := os.Stat(src)
	assert.True(t, os.IsNotExist(err))
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "moved", string(data))
}

func TestRemovePath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "file.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	require.NoError(t, RemovePath(filepath.Join(dir, "nested")))
	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}
