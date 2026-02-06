package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHuggingFaceScriptPath(t *testing.T) {
	// Test with existing script
	scriptDir, err := getHuggingFaceScriptPath("huggingface_download.py")
	if err != nil {
		// If script doesn't exist, skip the test
		t.Skipf("Python script not found, skipping test: %v", err)
	}
	assert.NoError(t, err)
	assert.NotEmpty(t, scriptDir)

	// Verify the directory exists
	_, err = os.Stat(scriptDir)
	assert.NoError(t, err)
	assert.True(t, filepath.IsAbs(scriptDir))

	// Verify the script file exists in that directory
	scriptPath := filepath.Join(scriptDir, "huggingface_download.py")
	_, err = os.Stat(scriptPath)
	assert.NoError(t, err)
}

func TestGetHuggingFaceScriptPath_NonExistentScript(t *testing.T) {
	scriptDir, err := getHuggingFaceScriptPath("non_existent_script.py")
	assert.Error(t, err)
	assert.Empty(t, scriptDir)
	assert.Contains(t, err.Error(), "Python script not found")
}

func TestGetHuggingFaceScriptPath_UploadScript(t *testing.T) {
	// Test with upload script
	scriptDir, err := getHuggingFaceScriptPath("huggingface_upload.py")
	if err != nil {
		// If script doesn't exist, skip the test
		t.Skipf("Python script not found, skipping test: %v", err)
	}
	assert.NoError(t, err)
	assert.NotEmpty(t, scriptDir)

	// Verify the directory exists
	_, err = os.Stat(scriptDir)
	assert.NoError(t, err)
	assert.True(t, filepath.IsAbs(scriptDir))

	// Verify the script file exists in that directory
	scriptPath := filepath.Join(scriptDir, "huggingface_upload.py")
	_, err = os.Stat(scriptPath)
	assert.NoError(t, err)
}

func TestGetHuggingFaceScriptPath_ReturnsDirectory(t *testing.T) {
	// Get the directory where this test file is located
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("Could not determine test file location")
	}
	testDir := filepath.Dir(testFile)

	// Test that the function returns the directory, not the file path
	scriptDir, err := getHuggingFaceScriptPath("huggingface_download.py")
	if err != nil {
		t.Skipf("Python script not found, skipping test: %v", err)
	}

	// The returned path should be a directory, and should match the test file's directory
	assert.Equal(t, testDir, scriptDir)
	assert.True(t, filepath.IsAbs(scriptDir))
}
