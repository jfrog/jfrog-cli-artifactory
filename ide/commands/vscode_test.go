package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVscodeCommand(t *testing.T) {
	serviceURL := "https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery"
	productPath := "/custom/path/product.json"
	repoKey := "repo"

	cmd := NewVscodeCommand(repoKey, productPath, serviceURL)

	assert.Equal(t, serviceURL, cmd.serviceURL)
	assert.Equal(t, productPath, cmd.productPath)
	assert.Equal(t, repoKey, cmd.repoKey)
}

func TestVscodeCommand_CommandName(t *testing.T) {
	cmd := NewVscodeCommand("", "", "")
	assert.Equal(t, "rt_vscode_config", cmd.CommandName())
}

func TestVscodeCommand_SetServerDetails(t *testing.T) {
	cmd := NewVscodeCommand("", "", "")
	serverDetails := &config.ServerDetails{
		Url:            "https://company.jfrog.io",
		ArtifactoryUrl: "https://company.jfrog.io/artifactory",
		AccessToken:    "test-token",
	}

	result := cmd.SetServerDetails(serverDetails)

	assert.Equal(t, serverDetails, cmd.serverDetails)
	assert.Equal(t, cmd, result) // Should return self for chaining
}

func TestVscodeCommand_ServerDetails(t *testing.T) {
	cmd := NewVscodeCommand("", "", "")
	serverDetails := &config.ServerDetails{
		Url:            "https://company.jfrog.io",
		ArtifactoryUrl: "https://company.jfrog.io/artifactory",
		AccessToken:    "test-token",
	}

	cmd.SetServerDetails(serverDetails)

	details, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, details)
}

func TestVscodeCommand_DetectVSCodeInstallation(t *testing.T) {
	// This test verifies that the detection method runs without error
	// but skips actual file checks since they depend on the environment
	cmd := NewVscodeCommand("", "", "")

	// The method should not panic and should handle missing installations gracefully
	_, err := cmd.detectVSCodeInstallation()
	// We expect an error since VSCode likely isn't installed in the test environment
	// But the method should handle it gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "VSCode")
	}
}

func TestVscodeCommand_CheckWritePermissions_NonExistentFile(t *testing.T) {
	cmd := NewVscodeCommand("", "/non/existent/path/product.json", "")

	err := cmd.checkWritePermissions()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to access product.json")
}

func TestVscodeCommand_CreateBackup(t *testing.T) {
	// Create temporary product.json file
	tempDir := t.TempDir()
	productPath := filepath.Join(tempDir, "product.json")
	originalContent := []byte(`{"extensionsGallery": {"serviceUrl": "https://marketplace.visualstudio.com"}}`)

	err := os.WriteFile(productPath, originalContent, 0644)
	require.NoError(t, err)

	cmd := NewVscodeCommand("", productPath, "")

	err = cmd.createBackup()
	assert.NoError(t, err)

	// Verify backup was created (it's in JFrog backup directory, not same directory)
	assert.NotEmpty(t, cmd.backupPath)
	assert.FileExists(t, cmd.backupPath)

	// Verify backup content
	backupContent, err := os.ReadFile(cmd.backupPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, backupContent)
}

func TestVscodeCommand_RestoreBackup(t *testing.T) {
	// Create temporary files
	tempDir := t.TempDir()
	productPath := filepath.Join(tempDir, "product.json")
	backupPath := productPath + ".backup"

	// Create original backup content
	originalContent := []byte(`{"extensionsGallery": {"serviceUrl": "https://marketplace.visualstudio.com"}}`)
	err := os.WriteFile(backupPath, originalContent, 0644)
	require.NoError(t, err)

	// Create modified product.json
	modifiedContent := []byte(`{"extensionsGallery": {"serviceUrl": "https://company.jfrog.io"}}`)
	err = os.WriteFile(productPath, modifiedContent, 0644)
	require.NoError(t, err)

	cmd := NewVscodeCommand("", productPath, "")
	cmd.backupPath = backupPath

	err = cmd.restoreBackup()
	assert.NoError(t, err)

	// Verify restoration
	restoredContent, err := os.ReadFile(productPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, restoredContent)
}

func TestVscodeCommand_ModifyProductJson_ValidFile(t *testing.T) {
	// Create temporary product.json file
	tempDir := t.TempDir()
	productPath := filepath.Join(tempDir, "product.json")

	// Create a valid product.json
	productContent := map[string]interface{}{
		"extensionsGallery": map[string]interface{}{
			"serviceUrl": "https://marketplace.visualstudio.com/_apis/public/gallery",
		},
		"nameShort": "Code",
		"version":   "1.70.0",
	}

	jsonData, err := json.Marshal(productContent)
	require.NoError(t, err)

	err = os.WriteFile(productPath, jsonData, 0644)
	require.NoError(t, err)

	cmd := NewVscodeCommand("", productPath, "")
	newServiceURL := "https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery"

	err = cmd.modifyProductJson(newServiceURL)
	assert.NoError(t, err)

	// Verify backup was created (in JFrog backup directory)
	assert.NotEmpty(t, cmd.backupPath)
	assert.FileExists(t, cmd.backupPath)
}

func TestVscodeCommand_ModifyProductJson_NonExistentFile(t *testing.T) {
	cmd := NewVscodeCommand("", "/non/existent/path/product.json", "")
	newServiceURL := "https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery"

	err := cmd.modifyProductJson(newServiceURL)
	assert.Error(t, err)
}

func TestVscodeCommand_GetManualSetupInstructions(t *testing.T) {
	cmd := NewVscodeCommand("", "", "")
	serviceURL := "https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery"

	instructions := cmd.getManualSetupInstructions(serviceURL)

	assert.NotEmpty(t, instructions)
	assert.Contains(t, instructions, serviceURL)
	assert.Contains(t, instructions, "product.json")

	// Should contain platform-specific instructions
	switch runtime.GOOS {
	case "darwin":
		assert.Contains(t, instructions, "Applications")
	case "windows":
		assert.Contains(t, instructions, "resources")
	case "linux":
		assert.Contains(t, instructions, "usr/share")
	}
}

func TestVscodeCommand_ValidateRepository_NoServerDetails(t *testing.T) {
	cmd := NewVscodeCommand("", "", "repo")

	// Should return error when no server details are set
	err := cmd.validateRepository()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server details not configured")
}

func TestVscodeCommand_HandlePermissionError_macOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	cmd := NewVscodeCommand("https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery",
		"/Applications/Visual Studio Code.app/Contents/Resources/app/product.json", "")

	err := cmd.handlePermissionError()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sudo")
	assert.Contains(t, err.Error(), "elevated privileges")
	assert.Contains(t, err.Error(), "/Applications/")
}

// Benchmark tests for performance
func BenchmarkVscodeCommand_DetectVSCodeInstallation(b *testing.B) {
	cmd := NewVscodeCommand("", "", "")

	for i := 0; i < b.N; i++ {
		_, _ = cmd.detectVSCodeInstallation()
	}
}
