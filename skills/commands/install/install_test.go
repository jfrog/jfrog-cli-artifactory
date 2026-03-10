package install

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnzipFile(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"SKILL.md":        "---\nname: test\n---",
		"main.py":         "print('hello')",
		"utils/helper.py": "pass",
	})

	err := unzipFile(zipPath, destDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: test")

	data, err = os.ReadFile(filepath.Join(destDir, "main.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('hello')", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "utils", "helper.py"))
	require.NoError(t, err)
	assert.Equal(t, "pass", string(data))
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644))

	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data))

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data))
}

func TestGetDestDir(t *testing.T) {
	cmd := NewInstallCommand().SetSlug("my-skill")

	assert.Equal(t, filepath.Join(".", "my-skill"), cmd.getDestDir())

	cmd.SetInstallPath("/custom/path")
	assert.Equal(t, filepath.Join("/custom/path", "my-skill"), cmd.getDestDir())
}

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	w := zip.NewWriter(f)
	defer func() {
		_ = w.Close()
	}()

	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}
