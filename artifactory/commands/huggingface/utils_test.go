package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPythonScripts(t *testing.T) {
	tmpDir, err := extractPythonScripts()
	require.NoError(t, err)
	defer func() { assert.NoError(t, os.RemoveAll(tmpDir)) }()

	assert.NotEmpty(t, tmpDir)
	assert.True(t, filepath.IsAbs(tmpDir))

	for _, script := range []string{"huggingface_download.py", "huggingface_upload.py"} {
		scriptPath := filepath.Join(tmpDir, script)
		info, err := os.Stat(scriptPath)
		assert.NoError(t, err, "script %s should exist", script)
		assert.Greater(t, info.Size(), int64(0), "script %s should not be empty", script)
	}
}
