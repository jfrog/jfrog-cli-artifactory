package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnzipFile(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "test.zip")
	testutil.CreateTestZip(t, zipPath, map[string]string{
		"SKILL.md":        "---\nname: test\n---",
		"main.py":         "print('hello')",
		"utils/helper.py": "pass",
	})

	err := UnzipFile(zipPath, destDir)
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

func TestUnzipFile_RejectsPathTraversal(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "evil.zip")
	testutil.CreateTestZip(t, zipPath, map[string]string{
		"../../outside.txt": "escaped",
	})

	err := UnzipFile(zipPath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal file path in zip")
	assert.Contains(t, err.Error(), "../../outside.txt")

	_, err = os.Stat(filepath.Join(filepath.Dir(destDir), "outside.txt"))
	assert.True(t, os.IsNotExist(err))
}
