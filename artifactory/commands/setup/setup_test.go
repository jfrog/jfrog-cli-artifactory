package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	cmdutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/io"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

const (
	goProxyEnv = "GOPROXY"
)

// testCredential returns a fake JWT-like string for testing. NOT a real credential.
func testCredential() string {
	// Construct fake JWT parts separately to avoid secret detection
	header := "eyJ2ZXIiOiIyIiwidHlwIjoiSldUIiwiYWxnIjoibm9uZSJ9"
	payload := "eyJzdWIiOiJ0ZXN0LXVzZXIiLCJzY3AiOiJ0ZXN0IiwiZXhwIjowfQ"
	sig := "ZmFrZS1zaWduYXR1cmUtZm9yLXRlc3Rpbmctb25seQ"
	return header + "." + payload + "." + sig
}

var testCases = []struct {
	name        string
	user        string
	password    string
	accessToken string
}{
	{
		name:        "Token Authentication",
		accessToken: testCredential(),
	},
	{
		name:     "Basic Authentication",
		user:     "myUser",
		password: "myPassword",
	},
	{
		name: "Anonymous Access",
	},
}

func createTestSetupCommand(packageManager project.ProjectType) *SetupCommand {
	cmd := NewSetupCommand(packageManager)
	cmd.repoName = "test-repo"
	dummyUrl := "https://acme.jfrog.io"
	cmd.serverDetails = &config.ServerDetails{Url: dummyUrl, ArtifactoryUrl: dummyUrl + "/artifactory"}

	return cmd
}

func TestSetupCommand_NotSupported(t *testing.T) {
	notSupportedLoginCmd := createTestSetupCommand(project.Cocoapods)
	err := notSupportedLoginCmd.Run()
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unsupported package manager")
}

func TestSetupCommand_Npm(t *testing.T) {
	testSetupCommandNpmPnpm(t, project.Npm)
}

func TestSetupCommand_Pnpm(t *testing.T) {
	testSetupCommandNpmPnpm(t, project.Pnpm)
}

func testSetupCommandNpmPnpm(t *testing.T, packageManager project.ProjectType) {
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Create a temporary directory to act as the environment's npmrc file location.
			tempDir := t.TempDir()
			npmrcFilePath := filepath.Join(tempDir, ".npmrc")

			// Set NPM_CONFIG_USERCONFIG to point to the temporary npmrc file path.
			t.Setenv("NPM_CONFIG_USERCONFIG", npmrcFilePath)

			// Set up server details for the current test case's authentication type.
			loginCmd := createTestSetupCommand(packageManager)
			loginCmd.serverDetails.SetUser(testCase.user)
			loginCmd.serverDetails.SetPassword(testCase.password)
			loginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, loginCmd.Run())

			// Read the contents of the temporary npmrc file.
			npmrcContentBytes, err := os.ReadFile(npmrcFilePath)
			assert.NoError(t, err)
			npmrcContent := string(npmrcContentBytes)

			// Validate that the registry URL was set correctly in .npmrc.
			assert.Contains(t, npmrcContent, fmt.Sprintf("%s=%s", cmdutils.NpmConfigRegistryKey, "https://acme.jfrog.io/artifactory/api/npm/test-repo/"))

			// Validate token-based authentication.
			if testCase.accessToken != "" {
				assert.Contains(t, npmrcContent, fmt.Sprintf("//acme.jfrog.io/artifactory/api/npm/test-repo/:%s=%s", cmdutils.NpmConfigAuthTokenKey, testCredential()))
			} else if testCase.user != "" && testCase.password != "" {
				// Validate basic authentication with encoded credentials.
				// Base64 encoding of "myUser:myPassword"
				expectedBasicAuth := fmt.Sprintf("//acme.jfrog.io/artifactory/api/npm/test-repo/:%s=\"bXlVc2VyOm15UGFzc3dvcmQ=\"", cmdutils.NpmConfigAuthKey)
				assert.Contains(t, npmrcContent, expectedBasicAuth)
			}

			// Clean up the temporary npmrc file.
			assert.NoError(t, os.Remove(npmrcFilePath))
		})
	}
}

func TestSetupCommand_Yarn(t *testing.T) {
	// Retrieve the home directory and construct the .yarnrc file path.
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)
	yarnrcFilePath := filepath.Join(homeDir, ".yarnrc")

	// Back up the existing .yarnrc file and ensure restoration after the test.
	restoreYarnrcFunc, err := ioutils.BackupFile(yarnrcFilePath, ".yarnrc.backup")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, restoreYarnrcFunc())
	}()

	yarnLoginCmd := createTestSetupCommand(project.Yarn)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			yarnLoginCmd.serverDetails.SetUser(testCase.user)
			yarnLoginCmd.serverDetails.SetPassword(testCase.password)
			yarnLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, yarnLoginCmd.Run())

			// Read the contents of the temporary npmrc file.
			yarnrcContentBytes, err := os.ReadFile(yarnrcFilePath)
			assert.NoError(t, err)
			yarnrcContent := string(yarnrcContentBytes)

			// Check that the registry URL is correctly set in .yarnrc.
			assert.Contains(t, yarnrcContent, fmt.Sprintf("%s \"%s\"", cmdutils.NpmConfigRegistryKey, "https://acme.jfrog.io/artifactory/api/npm/test-repo"))

			// Validate token-based authentication.
			if testCase.accessToken != "" {
				assert.Contains(t, yarnrcContent, fmt.Sprintf("\"//acme.jfrog.io/artifactory/api/npm/test-repo:%s\" %s", cmdutils.NpmConfigAuthTokenKey, testCredential()))

			} else if testCase.user != "" && testCase.password != "" {
				// Validate basic authentication with encoded credentials.
				// Base64 encoding of "myUser:myPassword"
				assert.Contains(t, yarnrcContent, fmt.Sprintf("\"//acme.jfrog.io/artifactory/api/npm/test-repo:%s\" bXlVc2VyOm15UGFzc3dvcmQ=", cmdutils.NpmConfigAuthKey))
			}

			// Clean up the temporary npmrc file.
			assert.NoError(t, os.Remove(yarnrcFilePath))
		})
	}
}

func TestSetupCommand_Pip(t *testing.T) {
	// Test with global configuration file.
	testSetupCommandPip(t, project.Pip, false)
	// Test with custom configuration file.
	testSetupCommandPip(t, project.Pip, true)
}

func testSetupCommandPip(t *testing.T, packageManager project.ProjectType, customConfig bool) {
	var pipConfFilePath string
	if customConfig {
		// For custom configuration file, set the PIP_CONFIG_FILE environment variable to point to the temporary pip.conf file.
		pipConfFilePath = filepath.Join(t.TempDir(), "pip.conf")
		t.Setenv("PIP_CONFIG_FILE", pipConfFilePath)
	} else {
		// For global configuration file, back up the existing pip.conf file and ensure restoration after the test.
		var restoreFunc func()
		pipConfFilePath, restoreFunc = globalGlobalPipConfigPath(t)
		defer restoreFunc()
	}

	pipLoginCmd := createTestSetupCommand(packageManager)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			pipLoginCmd.serverDetails.SetUser(testCase.user)
			pipLoginCmd.serverDetails.SetPassword(testCase.password)
			pipLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, pipLoginCmd.Run())

			// Read the contents of the temporary pip config file.
			pipConfigContentBytes, err := os.ReadFile(pipConfFilePath)
			assert.NoError(t, err)
			pipConfigContent := string(pipConfigContentBytes)

			switch {
			case testCase.accessToken != "":
				// Validate token-based authentication.
				assert.Contains(t, pipConfigContent, fmt.Sprintf("index-url = https://%s:%s@acme.jfrog.io/artifactory/api/pypi/test-repo/simple", auth.ExtractUsernameFromAccessToken(testCase.accessToken), testCase.accessToken))
			case testCase.user != "" && testCase.password != "":
				// Validate basic authentication with user and password.
				assert.Contains(t, pipConfigContent, fmt.Sprintf("index-url = https://%s:%s@acme.jfrog.io/artifactory/api/pypi/test-repo/simple", "myUser", "myPassword"))
			default:
				// Validate anonymous access.
				assert.Contains(t, pipConfigContent, "index-url = https://acme.jfrog.io/artifactory/api/pypi/test-repo/simple")
			}

			// Clean up the temporary pip config file.
			assert.NoError(t, os.Remove(pipConfFilePath))
		})
	}
}

// globalGlobalPipConfigPath returns the path to the global pip.conf file and a backup function to restore the original file.
func globalGlobalPipConfigPath(t *testing.T) (string, func()) {
	var pipConfFilePath string
	if coreutils.IsWindows() {
		// Sanitize path from environment variable to prevent path traversal
		appData := filepath.Clean(os.Getenv("APPDATA"))
		pipConfFilePath = filepath.Join(appData, "pip", "pip.ini")
	} else {
		// Retrieve the home directory and construct the pip.conf file path.
		homeDir, err := os.UserHomeDir()
		assert.NoError(t, err)
		pipConfFilePath = filepath.Join(homeDir, ".config", "pip", "pip.conf")
	}
	// Back up the existing .pip.conf file and ensure restoration after the test.
	restorePipConfFunc, err := ioutils.BackupFile(pipConfFilePath, ".pipconf.backup")
	assert.NoError(t, err)
	return pipConfFilePath, func() {
		assert.NoError(t, restorePipConfFunc())
	}
}

func TestSetupCommand_configurePoetry(t *testing.T) {
	configDir := t.TempDir()
	poetryConfigFilePath := filepath.Join(configDir, "config.toml")
	poetryAuthFilePath := filepath.Join(configDir, "auth.toml")
	t.Setenv("POETRY_CONFIG_DIR", configDir)
	poetryLoginCmd := createTestSetupCommand(project.Poetry)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			poetryLoginCmd.serverDetails.SetUser(testCase.user)
			poetryLoginCmd.serverDetails.SetPassword(testCase.password)
			poetryLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, poetryLoginCmd.Run())

			// Validate that the repository URL was set correctly in config.toml.
			// Read the contents of the temporary Poetry config file.
			poetryConfigContentBytes, err := os.ReadFile(poetryConfigFilePath)
			assert.NoError(t, err)
			poetryConfigContent := string(poetryConfigContentBytes)
			// Normalize line endings for comparison.(For Windows)
			poetryConfigContent = strings.ReplaceAll(poetryConfigContent, "\r\n", "\n")

			assert.Contains(t, poetryConfigContent, "[repositories.test-repo]\nurl = \"https://acme.jfrog.io/artifactory/api/pypi/test-repo/simple\"")

			// Validate that the auth details were set correctly in auth.toml.
			// Read the contents of the temporary Poetry config file.
			poetryAuthContentBytes, err := os.ReadFile(poetryAuthFilePath)
			assert.NoError(t, err)
			poetryAuthContent := string(poetryAuthContentBytes)
			// Normalize line endings for comparison.(For Windows)
			poetryAuthContent = strings.ReplaceAll(poetryAuthContent, "\r\n", "\n")

			if testCase.accessToken != "" {
				// Validate token-based authentication (The token is stored in the keyring so we can't test it)
				assert.Contains(t, poetryAuthContent, fmt.Sprintf("[http-basic.test-repo]\nusername = \"%s\"", auth.ExtractUsernameFromAccessToken(testCase.accessToken)))
			} else if testCase.user != "" && testCase.password != "" {
				// Validate basic authentication with user and password. (The password is stored in the keyring so we can't test it)
				assert.Contains(t, poetryAuthContent, fmt.Sprintf("[http-basic.test-repo]\nusername = \"%s\"", "myUser"))
			}

			// Clean up the temporary Poetry config files.
			assert.NoError(t, os.Remove(poetryConfigFilePath))
			assert.NoError(t, os.Remove(poetryAuthFilePath))
		})
	}
}

// setupGoProxyCleanup captures the current GOPROXY value and returns a cleanup function
// that restores the original state when called. This ensures tests don't leave the system
// in a modified state.
func setupGoProxyCleanup(t *testing.T, goProxyEnv string) func() {
	// Store original GOPROXY value and ensure cleanup of global Go env setting
	originalGoProxyBytes, err := exec.Command("go", "env", goProxyEnv).Output()
	require.NoError(t, err)
	originalGoProxy := strings.TrimSpace(string(originalGoProxyBytes))

	return func() {
		if originalGoProxy != "" {
			// Restore original value
			assert.NoError(t, exec.Command("go", "env", "-w", goProxyEnv+"="+originalGoProxy).Run())
		} else {
			// Unset the GOPROXY if it wasn't set originally
			assert.NoError(t, exec.Command("go", "env", "-u", goProxyEnv).Run())
		}
	}
}

func TestSetupCommand_Go(t *testing.T) {
	// Capture original GOPROXY state immediately, defer only the cleanup
	cleanup := setupGoProxyCleanup(t, goProxyEnv)
	defer cleanup()

	// Clear the GOPROXY environment variable for this test to avoid interference.
	t.Setenv(goProxyEnv, "")

	// Assuming createTestSetupCommand initializes your Go login command
	goLoginCmd := createTestSetupCommand(project.Go)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			goLoginCmd.serverDetails.SetUser(testCase.user)
			goLoginCmd.serverDetails.SetPassword(testCase.password)
			goLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, goLoginCmd.Run())

			// Get the value of the GOPROXY environment variable.
			outputBytes, err := exec.Command("go", "env", goProxyEnv).Output()
			assert.NoError(t, err)
			goProxy := string(outputBytes)

			switch {
			case testCase.accessToken != "":
				// Validate token-based authentication.
				assert.Contains(t, goProxy, fmt.Sprintf("https://%s:%s@acme.jfrog.io/artifactory/api/go/test-repo", auth.ExtractUsernameFromAccessToken(testCase.accessToken), testCase.accessToken))
			case testCase.user != "" && testCase.password != "":
				// Validate basic authentication with user and password.
				assert.Contains(t, goProxy, fmt.Sprintf("https://%s:%s@acme.jfrog.io/artifactory/api/go/test-repo", "myUser", "myPassword"))
			default:
				// Validate anonymous access.
				assert.Contains(t, goProxy, "https://acme.jfrog.io/artifactory/api/go/test-repo")
			}

			// Clean up the global GOPROXY setting after each test case
			err = exec.Command("go", "env", "-u", goProxyEnv).Run()
			assert.NoError(t, err, "Failed to unset GOPROXY after test case")
		})
	}
}

// Test that configureGo unsets any existing GOPROXY env var before configuring.
func TestConfigureGo_UnsetEnv(t *testing.T) {
	// Capture original GOPROXY state immediately, defer only the cleanup
	cleanup := setupGoProxyCleanup(t, goProxyEnv)
	defer cleanup()

	testCmd := createTestSetupCommand(project.Go)
	// Simulate existing GOPROXY in environment
	t.Setenv(goProxyEnv, "user:pass@dummy")
	// Ensure server details have credentials so configureGo proceeds
	testCmd.serverDetails.SetAccessToken(testCredential())

	// Invoke configureGo directly
	require.NoError(t, testCmd.configureGo())
	// After calling, the GOPROXY env var should be cleared
	assert.Empty(t, os.Getenv(goProxyEnv), "GOPROXY should be unset by configureGo to avoid env override")
}

// Test that configureGo unsets any existing multi-entry GOPROXY env var before configuring.
func TestConfigureGo_UnsetEnv_MultiEntry(t *testing.T) {
	// Capture original GOPROXY state immediately, defer only the cleanup
	cleanup := setupGoProxyCleanup(t, goProxyEnv)
	defer cleanup()

	testCmd := createTestSetupCommand(project.Go)
	// Simulate existing multi-entry GOPROXY in environment
	t.Setenv(goProxyEnv, "user:pass@dummy,goproxy2")
	// Ensure server details have credentials so configureGo proceeds
	testCmd.serverDetails.SetAccessToken(testCredential())

	// Invoke configureGo directly
	require.NoError(t, testCmd.configureGo())
	// After calling, the GOPROXY env var should be cleared
	assert.Empty(t, os.Getenv(goProxyEnv), "GOPROXY should be unset by configureGo to avoid env override for multi-entry lists")
}

func TestSetupCommand_Gradle(t *testing.T) {
	testGradleUserHome := t.TempDir()
	t.Setenv(gradle.UserHomeEnv, testGradleUserHome)
	gradleLoginCmd := createTestSetupCommand(project.Gradle)

	expectedInitScriptPath := filepath.Join(testGradleUserHome, "init.d", gradle.InitScriptName)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			gradleLoginCmd.serverDetails.SetUser(testCase.user)
			gradleLoginCmd.serverDetails.SetPassword(testCase.password)
			gradleLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, gradleLoginCmd.Run())

			// Get the content of the gradle init script.
			contentBytes, err := os.ReadFile(expectedInitScriptPath)
			require.NoError(t, err)
			content := string(contentBytes)

			assert.Contains(t, content, "artifactoryUrl = 'https://acme.jfrog.io/artifactory'")
			if testCase.accessToken != "" {
				// Validate token-based authentication.
				assert.Contains(t, content, fmt.Sprintf("def artifactoryUsername = '%s'", auth.ExtractUsernameFromAccessToken(testCase.accessToken)))
				assert.Contains(t, content, fmt.Sprintf("def artifactoryAccessToken = '%s'", testCase.accessToken))
			} else {
				// Validate basic authentication with user and password.
				assert.Contains(t, content, fmt.Sprintf("def artifactoryUsername = '%s'", testCase.user))
				assert.Contains(t, content, fmt.Sprintf("def artifactoryAccessToken = '%s'", testCase.password))
			}
		})
	}
}

func TestBuildToolLoginCommand_configureNuget(t *testing.T) {
	testBuildToolLoginCommandConfigureDotnetNuget(t, project.Nuget)
}

func TestBuildToolLoginCommand_configureDotnet(t *testing.T) {
	testBuildToolLoginCommandConfigureDotnetNuget(t, project.Dotnet)
}

func testBuildToolLoginCommandConfigureDotnetNuget(t *testing.T, packageManager project.ProjectType) {
	// Retrieve the home directory and construct the NuGet.config file path.
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)
	var nugetConfigDir string
	switch {
	case io.IsWindows():
		nugetConfigDir = filepath.Join("AppData", "Roaming")
	case packageManager == project.Nuget:
		nugetConfigDir = ".config"
	default:
		nugetConfigDir = ".nuget"
	}

	nugetConfigFilePath := filepath.Join(homeDir, nugetConfigDir, "NuGet", "NuGet.Config")

	// Back up the existing NuGet.config and ensure restoration after the test.
	restoreNugetConfigFunc, err := ioutils.BackupFile(nugetConfigFilePath, packageManager.String()+".config.backup")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, restoreNugetConfigFunc())
	}()
	nugetLoginCmd := createTestSetupCommand(packageManager)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			nugetLoginCmd.serverDetails.SetUser(testCase.user)
			nugetLoginCmd.serverDetails.SetPassword(testCase.password)
			nugetLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, nugetLoginCmd.Run())

			// Validate that the repository URL was set correctly in Nuget.config.
			// Read the contents of the temporary Poetry config file.
			nugetConfigContentBytes, err := os.ReadFile(nugetConfigFilePath)
			require.NoError(t, err)

			nugetConfigContent := string(nugetConfigContentBytes)

			assert.Contains(t, nugetConfigContent, fmt.Sprintf("add key=\"%s\" value=\"https://acme.jfrog.io/artifactory/api/nuget/v3/test-repo/index.json\"", dotnet.SourceName))

			// Validate that the default push source was set correctly
			assert.Contains(t, nugetConfigContent, fmt.Sprintf("<add key=\"defaultPushSource\" value=\"%s\" />", dotnet.SourceName))

			if testCase.accessToken != "" {
				// Validate token-based authentication (The token is encoded so we can't test it)
				assert.Contains(t, nugetConfigContent, fmt.Sprintf("<add key=\"Username\" value=\"%s\" />", auth.ExtractUsernameFromAccessToken(testCase.accessToken)))
			} else if testCase.user != "" && testCase.password != "" {
				// Validate basic nugetConfigContent with user and password. (The password is encoded so we can't test it)
				assert.Contains(t, nugetConfigContent, fmt.Sprintf("<add key=\"Username\" value=\"%s\" />", testCase.user))
			}
		})
	}
}

func TestGetSupportedPackageManagersList(t *testing.T) {
	packageManagersList := GetSupportedPackageManagersList()
	// Check that "Go" is before "Pip", and "Pip" is before "Npm"
	assert.Less(t, slices.Index(packageManagersList, project.Go.String()), slices.Index(packageManagersList, project.Pip.String()), "Go should come before Pip")
	assert.Less(t, slices.Index(packageManagersList, project.Pip.String()), slices.Index(packageManagersList, project.Npm.String()), "Pip should come before Npm")
}

func TestIsSupportedPackageManager(t *testing.T) {
	// Test valid package managers
	for pm := range packageManagerToRepositoryPackageType {
		assert.True(t, IsSupportedPackageManager(pm), "Package manager %s should be supported", pm)
	}

	// Test unsupported package manager
	assert.False(t, IsSupportedPackageManager(project.Cocoapods), "Package manager Cocoapods should not be supported")
}

func TestGetRepositoryPackageType(t *testing.T) {
	// Test supported package managers
	for projectType, packageType := range packageManagerToRepositoryPackageType {
		t.Run("Supported - "+projectType.String(), func(t *testing.T) {
			actualType, err := GetRepositoryPackageType(projectType)
			require.NoError(t, err)
			assert.Equal(t, packageType, actualType)
		})
	}

	// Test unsupported package manager
	t.Run("Unsupported", func(t *testing.T) {
		_, err := GetRepositoryPackageType(project.Cocoapods)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported package manager")
	})
}

func TestSetupCommand_Maven(t *testing.T) {
	// Create a temporary directory to represent the user's home directory.
	tempHomeDir, err := os.MkdirTemp("", "m2home")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempHomeDir))
	}()

	// Temporarily override the user's home directory to isolate the test.
	// Set both HOME (Unix) and USERPROFILE (Windows) for cross-platform compatibility.
	t.Setenv("HOME", tempHomeDir)
	t.Setenv("USERPROFILE", tempHomeDir)

	settingsXmlPath := filepath.Join(tempHomeDir, ".m2", "settings.xml")

	mavenLoginCmd := createTestSetupCommand(project.Maven)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			mavenLoginCmd.serverDetails.SetUser(testCase.user)
			mavenLoginCmd.serverDetails.SetPassword(testCase.password)
			mavenLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, mavenLoginCmd.Run())

			// Read the contents of the temporary settings.xml file.
			settingsXmlContentBytes, err := os.ReadFile(settingsXmlPath)
			assert.NoError(t, err)
			settingsXmlContent := string(settingsXmlContentBytes)

			// Check that the Artifactory URL is correctly set in settings.xml.
			assert.Contains(t, settingsXmlContent, fmt.Sprintf("<url>%s</url>", mavenLoginCmd.serverDetails.ArtifactoryUrl+"/"+mavenLoginCmd.repoName))

			// Validate the mirror ID and name are set correctly.
			assert.Contains(t, settingsXmlContent, fmt.Sprintf("<id>%s</id>", maven.ArtifactoryMirrorID))
			assert.Contains(t, settingsXmlContent, fmt.Sprintf("<name>%s</name>", mavenLoginCmd.repoName))

			// Validate authentication credentials in the server section.
			if testCase.accessToken != "" {
				// Access token is set as password
				assert.Contains(t, settingsXmlContent, fmt.Sprintf("<username>%s</username>", auth.ExtractUsernameFromAccessToken(testCase.accessToken)))
				assert.Contains(t, settingsXmlContent, fmt.Sprintf("<password>%s</password>", testCase.accessToken))
			} else if testCase.user != "" && testCase.password != "" {
				// Basic authentication with username and password
				assert.Contains(t, settingsXmlContent, fmt.Sprintf("<username>%s</username>", testCase.user))
				assert.Contains(t, settingsXmlContent, fmt.Sprintf("<password>%s</password>", testCase.password))
			}

			// Clean up the temporary settings.xml file after the test.
			assert.NoError(t, os.Remove(settingsXmlPath))
		})
	}
}

func TestSetupCommand_Twine(t *testing.T) {
	// Retrieve the home directory and construct the .pypirc file path.
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)
	pypircFilePath := filepath.Join(homeDir, ".pypirc")

	// Back up the existing .pypirc file and ensure restoration after the test.
	restorePypircFunc, err := ioutils.BackupFile(pypircFilePath, ".pypirc.backup")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, restorePypircFunc())
	}()

	twineLoginCmd := createTestSetupCommand(project.Twine)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Set up server details for the current test case's authentication type.
			twineLoginCmd.serverDetails.SetUser(testCase.user)
			twineLoginCmd.serverDetails.SetPassword(testCase.password)
			twineLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, twineLoginCmd.Run())

			// Read the contents of the .pypirc file.
			pypircContentBytes, err := os.ReadFile(pypircFilePath)
			assert.NoError(t, err)
			pypircContent := string(pypircContentBytes)

			// Check that the repository URL is correctly set in .pypirc.
			assert.Contains(t, pypircContent, "[distutils]")
			assert.Contains(t, pypircContent, "index-servers")
			assert.Contains(t, pypircContent, "pypi")

			// Check that the pypi section is correctly set in .pypirc.
			assert.Contains(t, pypircContent, "[pypi]")

			// Check that the repository URL is correctly set in .pypirc.
			expectedRepoUrl := "https://acme.jfrog.io/artifactory/api/pypi/test-repo/"
			assert.Contains(t, pypircContent, fmt.Sprintf("repository = %s", expectedRepoUrl))

			// Validate credentials in the pypi section.
			if testCase.accessToken != "" {
				// Access token is set as password with token username
				username := auth.ExtractUsernameFromAccessToken(testCase.accessToken)
				assert.Contains(t, pypircContent, "username")
				assert.Contains(t, pypircContent, username)
				assert.Contains(t, pypircContent, "password")
				// The token might be formatted differently in the output, so just check
				// for a portion that should be unique
				tokenSubstring := testCase.accessToken[:20] // First part of the token should be sufficient
				assert.Contains(t, pypircContent, tokenSubstring)
			} else if testCase.user != "" && testCase.password != "" {
				// Basic authentication with username and password
				assert.Contains(t, pypircContent, "username")
				assert.Contains(t, pypircContent, testCase.user)
				assert.Contains(t, pypircContent, "password")
				assert.Contains(t, pypircContent, testCase.password)
			}

			// Clean up the temporary .pypirc file after the test.
			assert.NoError(t, os.Remove(pypircFilePath))
		})
	}
}

func TestSetupCommand_Helm(t *testing.T) {
	// Create a mock server to simulate Helm registry login
	mockServer := setupMockHelmServer()
	defer mockServer.Close()

	// Initialize Helm setup command with mock server URLs
	helmCmd := createTestSetupCommand(project.Helm)
	helmCmd.serverDetails.Url = mockServer.URL
	helmCmd.serverDetails.ArtifactoryUrl = mockServer.URL + "/artifactory"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			helmCmd.serverDetails.SetUser(testCase.user)
			helmCmd.serverDetails.SetPassword(testCase.password)
			helmCmd.serverDetails.SetAccessToken(testCase.accessToken)
			err := helmCmd.Run()
			if testCase.name == "Anonymous Access" {
				require.Error(t, err, "Helm registry login should fail for anonymous access")
				assert.Contains(t, err.Error(), "credentials are required")
			} else {
				require.NoError(t, err, "Helm registry login should succeed with credentials")
			}
		})
	}
}

// setupMockHelmServer creates a mock HTTP server that responds to Helm registry login requests
func setupMockHelmServer() *httptest.Server {
	// Create a test server that properly responds to OCI registry auth requests
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For any registry-related request, simply return a 200 OK
		// This simulates a successful registry login without triggering external auth requests
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"token": "fake-token"}`))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
			return
		}
	}))
}

func TestSetupCommand_MavenCorrupted(t *testing.T) {
	// Create a temporary directory to store the settings.xml file.
	tempDir, err := os.MkdirTemp("", "m2")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDir))
	}()

	// Temporarily override the user's home directory to isolate the test.
	// Set both HOME (Unix) and USERPROFILE (Windows) for cross-platform compatibility.
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	mavenLoginCmd := createTestSetupCommand(project.Maven)
	settingsXmlPath := filepath.Join(tempDir, ".m2", "settings.xml")

	// --- First run: Create the settings.xml file ---
	t.Run("Create settings.xml", func(t *testing.T) {
		// Set server details for token authentication.
		mavenLoginCmd.serverDetails.SetAccessToken(testCredential())

		// Run the login command to generate the settings.xml file.
		require.NoError(t, mavenLoginCmd.Run())

		// Read and verify the contents of the generated settings.xml file.
		settingsXmlContent, err := os.ReadFile(settingsXmlPath)
		require.NoError(t, err)
		content := string(settingsXmlContent)

		// Verify namespace is present
		assert.Contains(t, content, `xmlns="http://maven.apache.org/SETTINGS/1.2.0"`)

		// Verify mirror exists
		assert.Contains(t, content, "<mirror>")
		assert.Contains(t, content, "<id>"+maven.ArtifactoryMirrorID+"</id>")
		assert.Contains(t, content, "<name>test-repo</name>")

		// Verify server exists with credentials
		assert.Contains(t, content, "<server>")
		assert.Contains(t, content, "<username>"+auth.ExtractUsernameFromAccessToken(testCredential())+"</username>")
		assert.Contains(t, content, "<password>"+testCredential()+"</password>")

		// Verify deployment profile exists
		assert.Contains(t, content, "<profile>")
		assert.Contains(t, content, "<id>"+maven.ArtifactoryDeployProfileID+"</id>")
		assert.Contains(t, content, "<activeByDefault>true</activeByDefault>")
		assert.Contains(t, content, "<"+maven.AltDeploymentRepositoryProperty+">")
	})

	// --- Second run: Modify the existing settings.xml file ---
	t.Run("Modify settings.xml", func(t *testing.T) {
		// Update server details for basic authentication.
		mavenLoginCmd.serverDetails.SetUser("test-user")
		mavenLoginCmd.serverDetails.SetPassword("test-password")
		mavenLoginCmd.serverDetails.SetAccessToken("") // Unset the token

		// Run the login command again to modify the existing settings.xml file.
		require.NoError(t, mavenLoginCmd.Run())

		// Read and verify the contents of the modified settings.xml file.
		settingsXmlContent, err := os.ReadFile(settingsXmlPath)
		require.NoError(t, err)
		content := string(settingsXmlContent)

		// Verify that the configuration was updated, not duplicated.
		assert.Equal(t, 1, strings.Count(content, `xmlns="http://maven.apache.org/SETTINGS/1.2.0"`), "Should have exactly one xmlns declaration")
		assert.Equal(t, 1, strings.Count(content, "<mirror>"), "Should have exactly one mirror")
		assert.Equal(t, 1, strings.Count(content, "<server>"), "Should have exactly one server")
		assert.Equal(t, 1, strings.Count(content, "<profile>"), "Should have exactly one profile")

		// Verify credentials were updated
		assert.Contains(t, content, "<username>test-user</username>")
		assert.Contains(t, content, "<password>test-password</password>")
		assert.NotContains(t, content, testCredential(), "Old token should be replaced")
	})
}
