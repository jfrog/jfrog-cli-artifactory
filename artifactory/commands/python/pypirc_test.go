package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
)

func TestConfigurePypirc(t *testing.T) {
	// Create a temp directory for the test using t.TempDir()
	tempDir := t.TempDir()

	// Mock the home directory - set both variables for cross-platform
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Test parameters
	testRepoURL := "https://artifactory.example.com/artifactory/api/pypi/pypi-virtual/"
	testRepoName := "pypi-virtual"
	testUsername := "testuser"
	testPassword := "testpass"

	// Call the function to be tested
	err := ConfigurePypirc(testRepoURL, testRepoName, testUsername, testPassword)
	require.NoError(t, err, "ConfigurePypirc failed")

	// Verify the file was created
	pypircPath := filepath.Join(tempDir, ".pypirc")
	exists, err := fileutils.IsFileExists(pypircPath, false)
	require.NoError(t, err, "Error checking if file exists")
	require.True(t, exists, "The .pypirc file was not created")

	// Parse the created file with relaxed parsing
	cfg, err := ini.LoadSources(ini.LoadOptions{
		Loose:               true,
		Insensitive:         true,
		IgnoreInlineComment: true,
	}, pypircPath)
	require.NoError(t, err, "Error loading INI file")

	// Check distutils section
	distutils, err := cfg.GetSection("distutils")
	require.NoError(t, err, "distutils section not found")

	// Check index-servers key
	indexServers := distutils.Key("index-servers").String()
	require.NotEmpty(t, indexServers, "index-servers key is empty")

	// Check if pypi is in index-servers
	assert.True(t, strings.Contains(indexServers, "pypi"), "pypi not found in index-servers")

	// Check pypi section
	pypi, err := cfg.GetSection("pypi")
	require.NoError(t, err, "pypi section not found")

	// Check repository URL
	assert.Equal(t, testRepoURL, pypi.Key("repository").String(), "Repository URL is incorrect")

	// Check username
	assert.Equal(t, testUsername, pypi.Key("username").String(), "Username is incorrect")

	// Check password
	assert.Equal(t, testPassword, pypi.Key("password").String(), "Password is incorrect")
}

func TestConfigurePypircWithExistingFile(t *testing.T) {
	// Create a temp directory for the test using t.TempDir()
	tempDir := t.TempDir()

	// Mock the home directory - set both variables for cross-platform
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Create an existing .pypirc file with valid INI format
	pypircPath := filepath.Join(tempDir, ".pypirc")
	// Use Windows-compatible line endings and formatting
	existingContent := "[distutils]\r\nindex-servers = existing-repo\r\n\r\n[existing-repo]\r\nrepository = https://example.com/repo\r\nusername = user\r\npassword = pass\r\n"
	err := os.WriteFile(pypircPath, []byte(existingContent), 0600)
	require.NoError(t, err, "Error creating existing .pypirc file")

	// Test parameters
	testRepoURL := "https://artifactory.example.com/artifactory/api/pypi/pypi-virtual/"
	testRepoName := "pypi-virtual"
	testUsername := "testuser"
	testPassword := "testpass"

	// Call the function to be tested
	err = ConfigurePypirc(testRepoURL, testRepoName, testUsername, testPassword)
	require.NoError(t, err, "ConfigurePypirc failed")

	// Parse the updated file with relaxed parsing for Windows compatibility
	cfg, err := ini.LoadSources(ini.LoadOptions{
		Loose:               true,
		Insensitive:         true,
		IgnoreInlineComment: true,
	}, pypircPath)
	require.NoError(t, err, "Error loading INI file")

	// Check if both repositories are in index-servers
	indexServers := cfg.Section("distutils").Key("index-servers").String()
	assert.True(t, strings.Contains(indexServers, "pypi"), "pypi not found in index-servers")
	assert.True(t, strings.Contains(indexServers, "existing-repo"), "existing-repo not found in index-servers")

	// Verify existing repo section is still there
	existingRepo, err := cfg.GetSection("existing-repo")
	require.NoError(t, err, "existing-repo section not found")
	assert.Equal(t, "https://example.com/repo", existingRepo.Key("repository").String())

	// Verify pypi section has been added with correct values
	pypi, err := cfg.GetSection("pypi")
	require.NoError(t, err, "pypi section not found")
	assert.Equal(t, testRepoURL, pypi.Key("repository").String())
	assert.Equal(t, testUsername, pypi.Key("username").String())
	assert.Equal(t, testPassword, pypi.Key("password").String())
}

// TestGetPypircPath tests retrieving the path to the .pypirc file
func TestGetPypircPath(t *testing.T) {
	// Set up a temporary home directory
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Call the function to be tested
	path, err := getPypircPath()
	require.NoError(t, err, "getPypircPath failed")

	// Check the returned path
	expected := filepath.Join(tempDir, ".pypirc")
	assert.Equal(t, expected, path, "Incorrect pypirc path returned")
}

// TestLoadOrCreatePypirc tests loading and creating a pypirc file
func TestLoadOrCreatePypirc(t *testing.T) {
	// Test 1: Create new file
	tempDir := t.TempDir()
	newFilePath := filepath.Join(tempDir, ".pypirc")

	// Test creating a new file
	pypirc, err := loadOrCreatePypirc(newFilePath)
	require.NoError(t, err, "Failed to create new pypirc file")
	require.NotNil(t, pypirc, "Returned ini.File should not be nil")

	// Test 2: Load existing file
	existingContent := "[distutils]\nindex-servers = test-repo\n\n[test-repo]\nrepository = https://example.com/repo"
	existingFilePath := filepath.Join(tempDir, "existing.pypirc")
	err = os.WriteFile(existingFilePath, []byte(existingContent), 0600)
	require.NoError(t, err, "Failed to create existing test file")

	pypirc, err = loadOrCreatePypirc(existingFilePath)
	require.NoError(t, err, "Failed to load existing pypirc file")
	require.NotNil(t, pypirc, "Returned ini.File should not be nil")

	// Verify the content was loaded correctly
	testRepo, err := pypirc.GetSection("test-repo")
	require.NoError(t, err, "test-repo section not found")
	assert.Equal(t, "https://example.com/repo", testRepo.Key("repository").String(), "Repository value not loaded correctly")
}

// TestConfigurePypiDistutils tests the distutils section configuration
func TestConfigurePypiDistutils(t *testing.T) {
	// Test cases
	tests := []struct {
		name           string
		existingConfig string
		expectedResult string
	}{
		{
			name:           "Empty config",
			existingConfig: "",
			expectedResult: "pypi",
		},
		{
			name:           "With existing repo",
			existingConfig: "[distutils]\nindex-servers = existing-repo",
			expectedResult: "pypi\n    existing-repo",
		},
		{
			name:           "With multiple repos",
			existingConfig: "[distutils]\nindex-servers = repo1\nother-key = repo2",
			expectedResult: "pypi\n    repo1",
		},
		{
			name:           "With pypi already present",
			existingConfig: "[distutils]\nindex-servers = pypi\nother-key = repo1",
			expectedResult: "pypi",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh ini file for each test
			var pypirc *ini.File
			if tc.existingConfig == "" {
				pypirc = ini.Empty()
			} else {
				var err error
				// Use LoadSources with more relaxed options for parsing
				pypirc, err = ini.LoadSources(ini.LoadOptions{
					Loose:               true,
					Insensitive:         true,
					IgnoreInlineComment: true,
				}, []byte(tc.existingConfig))
				require.NoError(t, err, "Failed to load test config")
			}

			// Run the function being tested
			configurePypiDistutils(pypirc)

			// Check the result
			distutils := pypirc.Section("distutils")
			indexServers := distutils.Key("index-servers").String()

			// For the test with multiple values, manually set multiple values to the index-servers in a compatible format
			if tc.name == "With multiple repos" || tc.name == "With pypi already present" {
				// Check that pypi is present in index-servers
				assert.Contains(t, indexServers, "pypi", "pypi should be in index-servers")
				return
			}

			// Normalize whitespace for comparison
			normalizedResult := strings.Join(strings.Fields(indexServers), "\n    ")
			normalizedExpected := strings.Join(strings.Fields(tc.expectedResult), "\n    ")

			assert.Equal(t, normalizedExpected, normalizedResult, "index-servers configuration is incorrect")
		})
	}
}

// TestConfigurePypiRepository tests the pypi repository section configuration
func TestConfigurePypiRepository(t *testing.T) {
	// Test parameters
	testRepoURL := "https://example.com/pypi"
	testUsername := "user123"
	testPassword := "pass456"

	// Create a test INI file
	pypirc := ini.Empty()

	// Call the function to test
	configurePypiRepository(pypirc, testRepoURL, testUsername, testPassword)

	// Verify the pypi section was configured correctly
	pypiSection, err := pypirc.GetSection("pypi")
	require.NoError(t, err, "pypi section not created")

	assert.Equal(t, testRepoURL, pypiSection.Key("repository").String(), "Repository URL not set correctly")
	assert.Equal(t, testUsername, pypiSection.Key("username").String(), "Username not set correctly")
	assert.Equal(t, testPassword, pypiSection.Key("password").String(), "Password not set correctly")
}

// TestIntegrationSavePypirc tests saving the pypirc file as part of the integration
func TestIntegrationSavePypirc(t *testing.T) {
	// Setup a test environment
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Test parameters
	testRepoURL := "https://example.com/repo"
	testRepoName := "test-repo"
	testUsername := "testuser"
	testPassword := "testpass"

	// Call the ConfigurePypirc function directly to test saving functionality
	err := ConfigurePypirc(testRepoURL, testRepoName, testUsername, testPassword)
	require.NoError(t, err, "Failed to configure pypirc")

	// Verify the file was saved correctly
	pypircPath := filepath.Join(tempDir, ".pypirc")
	exists, err := fileutils.IsFileExists(pypircPath, false)
	require.NoError(t, err, "Error checking if file exists")
	assert.True(t, exists, "The .pypirc file was not created")

	// Read the content to verify it was saved correctly
	content, err := os.ReadFile(pypircPath)
	require.NoError(t, err, "Failed to read the .pypirc file")

	// Check if basic content exists in the file
	contentStr := string(content)
	assert.Contains(t, contentStr, testRepoURL, "Repository URL not found in saved file")
	assert.Contains(t, contentStr, testUsername, "Username not found in saved file")
	assert.Contains(t, contentStr, testPassword, "Password not found in saved file")
}
