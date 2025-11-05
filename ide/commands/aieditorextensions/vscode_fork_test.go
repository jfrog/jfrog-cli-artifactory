package aieditorextensions

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

func TestGetVSCodeFork(t *testing.T) {
	// Test getting VSCode config
	vscConfig, exists := GetVSCodeFork("vscode")
	assert.True(t, exists)
	assert.Equal(t, "vscode", vscConfig.Name)
	assert.Equal(t, "Visual Studio Code", vscConfig.DisplayName)

	// Test getting Cursor config
	cursorConfig, exists := GetVSCodeFork("cursor")
	assert.True(t, exists)
	assert.Equal(t, "cursor", cursorConfig.Name)
	assert.Equal(t, "Cursor", cursorConfig.DisplayName)

	// Test getting Windsurf config
	windsurfConfig, exists := GetVSCodeFork("windsurf")
	assert.True(t, exists)
	assert.Equal(t, "windsurf", windsurfConfig.Name)
	assert.Equal(t, "Windsurf", windsurfConfig.DisplayName)

	// Test non-existent fork
	_, exists = GetVSCodeFork("nonexistent")
	assert.False(t, exists)
}

func TestVSCodeForkConfig_GetDefaultInstallPath(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	path := forkConfig.GetDefaultInstallPath()
	assert.NotEmpty(t, path, "Should return a default path for current OS")
}

func TestVSCodeForkConfig_GetAllInstallPaths(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	paths := forkConfig.GetAllInstallPaths()
	assert.NotEmpty(t, paths, "Should return paths for current OS")
}

func TestNewVSCodeForkCommand(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	serviceURL := "https://company.jfrog.io/artifactory/api/aieditorextension/repo/_apis/public/gallery"
	productPath := "/custom/path/product.json"
	repoKey := "repo"

	cmd := NewVSCodeForkCommand(forkConfig, repoKey, productPath, serviceURL)

	assert.Equal(t, serviceURL, cmd.serviceURL)
	assert.Equal(t, productPath, cmd.productPath)
	assert.Equal(t, repoKey, cmd.repoKey)
	assert.Equal(t, forkConfig, cmd.forkConfig)
}

func TestVSCodeForkCommand_CommandName(t *testing.T) {
	// Test VSCode command name
	vscConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(vscConfig, "", "", "")
	assert.Equal(t, "setup-vscode", cmd.CommandName())

	// Test Cursor command name
	cursorConfig, _ := GetVSCodeFork("cursor")
	cmd = NewVSCodeForkCommand(cursorConfig, "", "", "")
	assert.Equal(t, "setup-cursor", cmd.CommandName())

	// Test Windsurf command name
	windsurfConfig, _ := GetVSCodeFork("windsurf")
	cmd = NewVSCodeForkCommand(windsurfConfig, "", "", "")
	assert.Equal(t, "setup-windsurf", cmd.CommandName())
}

func TestVSCodeForkCommand_SetServerDetails(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "", "")
	serverDetails := &config.ServerDetails{
		Url:            "https://company.jfrog.io",
		ArtifactoryUrl: "https://company.jfrog.io/artifactory",
		AccessToken:    "test-token",
	}

	result := cmd.SetServerDetails(serverDetails)

	assert.Equal(t, serverDetails, cmd.serverDetails)
	assert.Equal(t, cmd, result) // Should return self for chaining
}

func TestVSCodeForkCommand_ServerDetails(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "", "")
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

func TestVSCodeForkCommand_DetectInstallation(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "", "")

	// On CI/CD environments, IDEs might not be installed
	// This test mainly verifies the method doesn't panic
	_, err := cmd.detectInstallation()

	// We don't assert error here as the IDE might not be installed in test environment
	// Just ensure the method runs without panic
	t.Logf("Detection result for %s: %v", forkConfig.DisplayName, err)
}

func TestVSCodeForkCommand_CheckWritePermissions_NonExistentFile(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "/path/that/does/not/exist/product.json", "")
	err := cmd.checkWritePermissions()
	assert.Error(t, err)
}

func TestVSCodeForkCommand_CreateBackup(t *testing.T) {
	// Create a temporary product.json file
	tmpDir := t.TempDir()
	productPath := filepath.Join(tmpDir, "product.json")
	content := []byte(`{"test": "data"}`)
	err := os.WriteFile(productPath, content, 0644)
	require.NoError(t, err)

	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", productPath, "")
	err = cmd.createBackup()
	require.NoError(t, err)

	// Verify backup exists
	assert.NotEmpty(t, cmd.backupPath)
	assert.FileExists(t, cmd.backupPath)

	// Verify backup content matches original
	backupContent, err := os.ReadFile(cmd.backupPath)
	require.NoError(t, err)
	assert.Equal(t, content, backupContent)
}

func TestVSCodeForkCommand_RestoreBackup(t *testing.T) {
	// Create a temporary directory and files
	tmpDir := t.TempDir()
	productPath := filepath.Join(tmpDir, "product.json")
	backupPath := filepath.Join(tmpDir, "product.json.backup")

	// Create original and backup files
	originalContent := []byte(`{"original": "data"}`)
	backupContent := []byte(`{"backup": "data"}`)

	err := os.WriteFile(productPath, originalContent, 0644)
	require.NoError(t, err)
	err = os.WriteFile(backupPath, backupContent, 0644)
	require.NoError(t, err)

	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", productPath, "")
	cmd.backupPath = backupPath

	err = cmd.restoreBackup()
	require.NoError(t, err)

	// Verify product.json now has backup content
	restoredContent, err := os.ReadFile(productPath)
	require.NoError(t, err)
	assert.Equal(t, backupContent, restoredContent)
}

func TestVSCodeForkCommand_ModifyProductJson_ValidFile(t *testing.T) {
	// Create a temporary product.json file with proper content
	tmpDir := t.TempDir()
	productPath := filepath.Join(tmpDir, "product.json")

	initialContent := map[string]interface{}{
		"nameShort": "Code",
		"nameLong":  "Visual Studio Code",
		"extensionsGallery": map[string]interface{}{
			"serviceUrl": "https://marketplace.visualstudio.com/_apis/public/gallery",
		},
	}

	content, err := json.MarshalIndent(initialContent, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(productPath, content, 0644)
	require.NoError(t, err)

	// Create command and modify
	newServiceURL := "https://company.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery"
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", productPath, newServiceURL)
	err = cmd.modifyProductJson()
	require.NoError(t, err)

	// Read and verify the modified file
	modifiedContent, err := os.ReadFile(productPath)
	require.NoError(t, err)

	var modifiedData map[string]interface{}
	err = json.Unmarshal(modifiedContent, &modifiedData)
	require.NoError(t, err)

	// Verify serviceUrl was updated
	gallery, ok := modifiedData["extensionsGallery"].(map[string]interface{})
	require.True(t, ok, "extensionsGallery should be a map")
	assert.Equal(t, newServiceURL, gallery["serviceUrl"])

	// Verify other fields were preserved
	assert.Equal(t, "Code", modifiedData["nameShort"])
	assert.Equal(t, "Visual Studio Code", modifiedData["nameLong"])
}

func TestVSCodeForkCommand_ModifyProductJson_NonExistentFile(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "/path/that/does/not/exist/product.json", "https://test.com")
	err := cmd.modifyProductJson()
	assert.Error(t, err)
}

func TestVSCodeForkCommand_GetManualSetupInstructions(t *testing.T) {
	serviceURL := "https://company.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery"
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", "", serviceURL)

	instructions := cmd.getManualSetupInstructions()

	assert.Contains(t, instructions, serviceURL)
	assert.Contains(t, instructions, "product.json")
	assert.Contains(t, instructions, "extensionsGallery")
}

func TestVSCodeForkCommand_ValidateRepository_NoServerDetails(t *testing.T) {
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "test-repo", "", "")
	err := cmd.validateRepository()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server details not configured")
}

func TestVSCodeForkCommand_HandlePermissionError_macOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("This test is specific to macOS")
	}

	serviceURL := "https://company.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery"
	productPath := "/Applications/Visual Studio Code.app/Contents/Resources/app/product.json"
	forkConfig, _ := GetVSCodeFork("vscode")
	cmd := NewVSCodeForkCommand(forkConfig, "", productPath, serviceURL)

	err := cmd.handlePermissionError()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient permissions")
	assert.Contains(t, err.Error(), "sudo")
}

func TestSetupVSCodeFork_AllForks(t *testing.T) {
	// Test that all registered forks can be retrieved
	forks := GetSupportedForks()
	assert.GreaterOrEqual(t, len(forks), 3, "Should have at least 3 forks (vscode, cursor, windsurf)")

	// Test that each fork has valid configuration
	for _, forkName := range forks {
		config, exists := GetVSCodeFork(forkName)
		assert.True(t, exists, "Fork %s should exist", forkName)
		assert.NotEmpty(t, config.Name)
		assert.NotEmpty(t, config.DisplayName)
		assert.NotEmpty(t, config.InstallPaths)
		assert.Equal(t, "product.json", config.ProductJson)
	}
}
