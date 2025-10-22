package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJetbrainsCommand(t *testing.T) {
	repositoryURL := "https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo"
	repoKey := "repo"

	cmd := NewJetbrainsCommand(repositoryURL, repoKey)

	assert.Equal(t, repositoryURL, cmd.repositoryURL)
	assert.Equal(t, repoKey, cmd.repoKey)
	assert.NotNil(t, cmd.backupPaths)
	assert.Equal(t, 0, len(cmd.backupPaths))
	assert.Equal(t, 0, len(cmd.detectedIDEs))
}

func TestJetbrainsCommand_CommandName(t *testing.T) {
	cmd := NewJetbrainsCommand("", "")
	assert.Equal(t, "rt_jetbrains_config", cmd.CommandName())
}

func TestJetbrainsCommand_SetServerDetails(t *testing.T) {
	cmd := NewJetbrainsCommand("", "")
	serverDetails := &config.ServerDetails{
		Url:            "https://company.jfrog.io",
		ArtifactoryUrl: "https://company.jfrog.io/artifactory",
		AccessToken:    "test-token",
	}

	result := cmd.SetServerDetails(serverDetails)

	assert.Equal(t, serverDetails, cmd.serverDetails)
	assert.Equal(t, cmd, result) // Should return self for chaining
}

func TestJetbrainsCommand_ServerDetails(t *testing.T) {
	cmd := NewJetbrainsCommand("", "")
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

func TestJetbrainsCommand_ParseIDEFromDirName(t *testing.T) {
	cmd := NewJetbrainsCommand("", "")

	testCases := []struct {
		dirName      string
		expectedName string
		expectedVer  string
		shouldParse  bool
	}{
		{"IntelliJIdea2023.3", "IntelliJ IDEA", "2023.3", true},
		{"PyCharm2023.3", "PyCharm", "2023.3", true},
		{"WebStorm2023.2.1", "WebStorm", "2023.2.1", true},
		{"PhpStorm2024.1", "PhpStorm", "2024.1", true},
		{"RubyMine2023.3", "RubyMine", "2023.3", true},
		{"CLion2023.3", "CLion", "2023.3", true},
		{"DataGrip2023.3", "DataGrip", "2023.3", true},
		{"GoLand2023.3", "GoLand", "2023.3", true},
		{"Rider2023.3", "Rider", "2023.3", true},
		{"AndroidStudio2023.1", "Android Studio", "2023.1", true},
		{"AppCode2023.3", "AppCode", "2023.3", true},
		{"RustRover2023.3", "RustRover", "2023.3", true},
		{"Aqua2023.3", "Aqua", "2023.3", true},
		{"UnknownIDE2023.3", "", "", false},
		{"invalidname", "", "", false},
		{"", "", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.dirName, func(t *testing.T) {
			ide := cmd.parseIDEFromDirName(tc.dirName)
			if tc.shouldParse {
				require.NotNil(t, ide)
				assert.Equal(t, tc.expectedName, ide.Name)
				assert.Equal(t, tc.expectedVer, ide.Version)
			} else {
				assert.Nil(t, ide)
			}
		})
	}
}

func TestJetbrainsCommand_DetectJetBrainsIDEs(t *testing.T) {
	// This test verifies that the detection method runs without error
	// but skips actual file checks since they depend on the environment
	cmd := NewJetbrainsCommand("", "")

	// The method should not panic and should handle missing installations gracefully
	err := cmd.detectJetBrainsIDEs()
	// We expect an error since JetBrains IDEs likely aren't installed in the test environment
	// But the method should handle it gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "JetBrains")
	}
}

func TestJetbrainsCommand_DetectJetBrainsIDEs_WithXDGConfigHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		// For testing purposes, we can't change runtime.GOOS, so skip on non-Linux
		t.Skip("XDG_CONFIG_HOME test is only fully testable on Linux")
	}

	// Create temporary directory for XDG_CONFIG_HOME
	tempDir := t.TempDir()
	xdgConfigHome := filepath.Join(tempDir, "config")

	// Create mock JetBrains configuration directory structure
	jetbrainsDir := filepath.Join(xdgConfigHome, "JetBrains")
	ideaDir := filepath.Join(jetbrainsDir, "IntelliJIdea2023.3")
	err := os.MkdirAll(ideaDir, 0755)
	require.NoError(t, err)

	// Create mock idea.properties file
	propertiesPath := filepath.Join(ideaDir, "idea.properties")
	err = os.WriteFile(propertiesPath, []byte("# Test properties\n"), 0644)
	require.NoError(t, err)

	// Set XDG_CONFIG_HOME environment variable
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if originalXDG != "" {
			if err := os.Setenv("XDG_CONFIG_HOME", originalXDG); err != nil {
				t.Logf("Warning: failed to restore XDG_CONFIG_HOME: %v", err)
			}
		} else {
			if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
				t.Logf("Warning: failed to unset XDG_CONFIG_HOME: %v", err)
			}
		}
	}()
	err = os.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	require.NoError(t, err)

	// Test detection
	cmd := NewJetbrainsCommand("", "")
	err = cmd.detectJetBrainsIDEs()
	assert.NoError(t, err, "Detection should succeed when XDG_CONFIG_HOME is set")
	assert.Len(t, cmd.detectedIDEs, 1, "Should detect one IDE")

	if len(cmd.detectedIDEs) > 0 {
		ide := cmd.detectedIDEs[0]
		assert.Equal(t, "IntelliJ IDEA", ide.Name)
		assert.Equal(t, "2023.3", ide.Version)
		assert.Equal(t, propertiesPath, ide.PropertiesPath)
		assert.Equal(t, ideaDir, ide.ConfigDir)
	}
}

func TestJetbrainsCommand_CreateBackup(t *testing.T) {
	// Create temporary idea.properties file
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	originalContent := []byte(`# IDE Configuration
ide.config.path=${user.home}/.config/JetBrains/IntelliJIdea2023.3
`)

	err := os.WriteFile(propertiesPath, originalContent, 0644)
	require.NoError(t, err)

	cmd := NewJetbrainsCommand("", "")
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err = cmd.createBackup(ide)
	assert.NoError(t, err)

	// Verify backup was created
	backupPath, exists := cmd.backupPaths[propertiesPath]
	assert.True(t, exists)
	assert.FileExists(t, backupPath)

	// Verify backup content
	backupContent, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, backupContent)
}

func TestJetbrainsCommand_CreateBackup_NonExistentFile(t *testing.T) {
	// Test backup creation when properties file doesn't exist
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")

	cmd := NewJetbrainsCommand("", "")
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err := cmd.createBackup(ide)
	assert.NoError(t, err)

	// Verify backup marker was created
	backupPath, exists := cmd.backupPaths[propertiesPath]
	assert.True(t, exists)
	assert.FileExists(t, backupPath)

	// Verify backup is marked as empty
	backupContent, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Contains(t, string(backupContent), "Empty properties file backup")
}

func TestJetbrainsCommand_RestoreBackup(t *testing.T) {
	// Create temporary files
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	backupPath := propertiesPath + ".backup"

	// Create original backup content
	originalContent := []byte(`# IDE Configuration
ide.config.path=${user.home}/.config/JetBrains/IntelliJIdea2023.3
`)
	err := os.WriteFile(backupPath, originalContent, 0644)
	require.NoError(t, err)

	// Create modified properties file
	modifiedContent := []byte(`# IDE Configuration
ide.config.path=${user.home}/.config/JetBrains/IntelliJIdea2023.3
idea.plugins.host=https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo
`)
	err = os.WriteFile(propertiesPath, modifiedContent, 0644)
	require.NoError(t, err)

	cmd := NewJetbrainsCommand("", "")
	cmd.backupPaths[propertiesPath] = backupPath
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err = cmd.restoreBackup(ide)
	assert.NoError(t, err)

	// Verify restoration
	restoredContent, err := os.ReadFile(propertiesPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, restoredContent)
}

func TestJetbrainsCommand_RestoreBackup_EmptyFile(t *testing.T) {
	// Test restoration of an empty file backup
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	backupPath := propertiesPath + ".backup"

	// Create empty file backup marker
	emptyBackupContent := []byte("# Empty properties file backup\n")
	err := os.WriteFile(backupPath, emptyBackupContent, 0644)
	require.NoError(t, err)

	// Create properties file
	modifiedContent := []byte(`# IDE Configuration
idea.plugins.host=https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo
`)
	err = os.WriteFile(propertiesPath, modifiedContent, 0644)
	require.NoError(t, err)

	cmd := NewJetbrainsCommand("", "")
	cmd.backupPaths[propertiesPath] = backupPath
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err = cmd.restoreBackup(ide)
	assert.NoError(t, err)

	// Verify file was removed
	_, err = os.Stat(propertiesPath)
	assert.True(t, os.IsNotExist(err))
}

func TestJetbrainsCommand_ModifyPropertiesFile_NewFile(t *testing.T) {
	// Test creating a new properties file
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	repositoryURL := "https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo"

	cmd := NewJetbrainsCommand("", "")
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err := cmd.modifyPropertiesFile(ide, repositoryURL)
	assert.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, propertiesPath)

	// Verify content
	content, err := os.ReadFile(propertiesPath)
	require.NoError(t, err)
	contentStr := string(content)
	assert.Contains(t, contentStr, "idea.plugins.host="+repositoryURL)
	assert.Contains(t, contentStr, "JFrog Artifactory plugins repository")
}

func TestJetbrainsCommand_ModifyPropertiesFile_ExistingFile(t *testing.T) {
	// Test modifying an existing properties file
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	repositoryURL := "https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo"

	// Create existing properties file
	originalContent := `# IDE Configuration
ide.config.path=${user.home}/.config/JetBrains/IntelliJIdea2023.3
ide.system.path=${user.home}/.local/share/JetBrains/IntelliJIdea2023.3
`
	err := os.WriteFile(propertiesPath, []byte(originalContent), 0644)
	require.NoError(t, err)

	cmd := NewJetbrainsCommand("", "")
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err = cmd.modifyPropertiesFile(ide, repositoryURL)
	assert.NoError(t, err)

	// Verify file was modified
	content, err := os.ReadFile(propertiesPath)
	require.NoError(t, err)
	contentStr := string(content)
	assert.Contains(t, contentStr, originalContent)
	assert.Contains(t, contentStr, "idea.plugins.host="+repositoryURL)
	assert.Contains(t, contentStr, "JFrog Artifactory plugins repository")
}

func TestJetbrainsCommand_ModifyPropertiesFile_UpdateExisting(t *testing.T) {
	// Test updating an existing idea.plugins.host entry
	tempDir := t.TempDir()
	propertiesPath := filepath.Join(tempDir, "idea.properties")
	repositoryURL := "https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo"

	// Create existing properties file with plugins host
	originalContent := `# IDE Configuration
ide.config.path=${user.home}/.config/JetBrains/IntelliJIdea2023.3
idea.plugins.host=https://old-repo.com/plugins
ide.system.path=${user.home}/.local/share/JetBrains/IntelliJIdea2023.3
`
	err := os.WriteFile(propertiesPath, []byte(originalContent), 0644)
	require.NoError(t, err)

	cmd := NewJetbrainsCommand("", "")
	ide := IDEInstallation{
		Name:           "IntelliJ IDEA",
		Version:        "2023.3",
		PropertiesPath: propertiesPath,
		ConfigDir:      tempDir,
	}

	err = cmd.modifyPropertiesFile(ide, repositoryURL)
	assert.NoError(t, err)

	// Verify file was modified
	content, err := os.ReadFile(propertiesPath)
	require.NoError(t, err)
	contentStr := string(content)
	assert.Contains(t, contentStr, "idea.plugins.host="+repositoryURL)
	assert.NotContains(t, contentStr, "https://old-repo.com/plugins")
}

func TestJetbrainsCommand_GetManualSetupInstructions(t *testing.T) {
	cmd := NewJetbrainsCommand("", "")
	repositoryURL := "https://company.jfrog.io/artifactory/api/jetbrainsplugins/repo"

	instructions := cmd.getManualSetupInstructions(repositoryURL)

	assert.NotEmpty(t, instructions)
	assert.Contains(t, instructions, repositoryURL)
	assert.Contains(t, instructions, "idea.properties")
	assert.Contains(t, instructions, "idea.plugins.host")

	// Should contain platform-specific instructions
	switch runtime.GOOS {
	case "darwin":
		assert.Contains(t, instructions, "Library/Application Support")
	case "windows":
		assert.Contains(t, instructions, "APPDATA")
	case "linux":
		assert.Contains(t, instructions, ".config")
	}

	// Should contain supported IDEs
	assert.Contains(t, instructions, "IntelliJ IDEA")
	assert.Contains(t, instructions, "PyCharm")
	assert.Contains(t, instructions, "WebStorm")
}

func TestJetbrainsCommand_ValidateRepository_NoServerDetails(t *testing.T) {
	cmd := NewJetbrainsCommand("", "repo")

	// Should return error when no server details are set
	err := cmd.validateRepository()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server details not configured")
}

// Benchmark tests for performance
func BenchmarkJetbrainsCommand_DetectJetBrainsIDEs(b *testing.B) {
	cmd := NewJetbrainsCommand("", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset detected IDEs
		cmd.detectedIDEs = nil
		_ = cmd.detectJetBrainsIDEs()
	}
}

func BenchmarkJetbrainsCommand_ParseIDEFromDirName(b *testing.B) {
	cmd := NewJetbrainsCommand("", "")
	testDirs := []string{
		"IntelliJIdea2023.3",
		"PyCharm2023.3",
		"WebStorm2023.2.1",
		"PhpStorm2024.1",
		"InvalidDir",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, dir := range testDirs {
			_ = cmd.parseIDEFromDirName(dir)
		}
	}
}

func TestJetbrainsCommand_DetectJetBrainsIDEs_WithTestAppData(t *testing.T) {
	// Test TEST_APPDATA support for Windows testing environments

	// Create temporary directory for TEST_APPDATA
	tempDir := t.TempDir()

	// Create mock JetBrains configuration directory structure
	jetbrainsDir := filepath.Join(tempDir, "JetBrains")
	ideaDir := filepath.Join(jetbrainsDir, "IntelliJIdea2023.3")
	err := os.MkdirAll(ideaDir, 0755)
	require.NoError(t, err)

	// Create mock idea.properties file
	propertiesPath := filepath.Join(ideaDir, "idea.properties")
	propertiesContent := "# Test properties\nide.system.path=${user.home}/.local/share/JetBrains/IntelliJIdea2023.3\n"
	err = os.WriteFile(propertiesPath, []byte(propertiesContent), 0644)
	require.NoError(t, err)

	// Set TEST_APPDATA environment variable
	originalTestAppData := os.Getenv("TEST_APPDATA")
	defer func() {
		if originalTestAppData != "" {
			_ = os.Setenv("TEST_APPDATA", originalTestAppData)
		} else {
			_ = os.Unsetenv("TEST_APPDATA")
		}
	}()
	err = os.Setenv("TEST_APPDATA", tempDir)
	require.NoError(t, err)

	// Create JetBrains command and test detection
	cmd := &JetbrainsCommand{}

	// For testing, temporarily modify runtime.GOOS to simulate Windows behavior
	// Note: We can't actually change runtime.GOOS, so this test documents the intended behavior
	if runtime.GOOS == "windows" {
		// Run detection - should find our mock IDE
		err = cmd.detectJetBrainsIDEs()
		// Should succeed when TEST_APPDATA points to our mock directory
		require.NoError(t, err)
		require.Len(t, cmd.detectedIDEs, 1)
		require.Equal(t, "IntelliJ IDEA", cmd.detectedIDEs[0].Name)
		require.Equal(t, "2023.3", cmd.detectedIDEs[0].Version)
	} else {
		// On non-Windows, TEST_APPDATA should not affect detection
		t.Logf("TEST_APPDATA test is primarily for Windows environments")
	}
}
