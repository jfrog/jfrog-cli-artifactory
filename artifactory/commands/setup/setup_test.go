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

// #nosec G101 -- Dummy token for tests
var dummyToken = "eyJ2ZXIiOiIyIiwidHlwIjoiSldUIiwiYWxnIjoiUlMyNTYiLCJraWQiOiJIcnU2VHctZk1yOTV3dy12TDNjV3ZBVjJ3Qm9FSHpHdGlwUEFwOE1JdDljIn0.eyJzdWIiOiJqZnJ0QDAxYzNnZmZoZzJlOHc2MTQ5ZTNhMnEwdzk3XC91c2Vyc1wvYWRtaW4iLCJzY3AiOiJtZW1iZXItb2YtZ3JvdXBzOnJlYWRlcnMgYXBpOioiLCJhdWQiOiJqZnJ0QDAxYzNnZmZoZzJlOHc2MTQ5ZTNhMnEwdzk3IiwiaXNzIjoiamZydEAwMWMzZ2ZmaGcyZTh3NjE0OWUzYTJxMHc5NyIsImV4cCI6MTU1NjAzNzc2NSwiaWF0IjoxNTU2MDM0MTY1LCJqdGkiOiI1M2FlMzgyMy05NGM3LTQ0OGItOGExOC1iZGVhNDBiZjFlMjAifQ.Bp3sdvppvRxysMlLgqT48nRIHXISj9sJUCXrm7pp8evJGZW1S9hFuK1olPmcSybk2HNzdzoMcwhUmdUzAssiQkQvqd_HanRcfFbrHeg5l1fUQ397ECES-r5xK18SYtG1VR7LNTVzhJqkmRd3jzqfmIK2hKWpEgPfm8DRz3j4GGtDRxhb3oaVsT2tSSi_VfT3Ry74tzmO0GcCvmBE2oh58kUZ4QfEsalgZ8IpYHTxovsgDx_M7ujOSZx_hzpz-iy268-OkrU22PQPCfBmlbEKeEUStUO9n0pj4l1ODL31AGARyJRy46w4yzhw7Fk5P336WmDMXYs5LAX2XxPFNLvNzA"

var testCases = []struct {
	name        string
	user        string
	password    string
	accessToken string
}{
	{
		name:        "Token Authentication",
		accessToken: dummyToken,
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
				assert.Contains(t, npmrcContent, fmt.Sprintf("//acme.jfrog.io/artifactory/api/npm/test-repo/:%s=%s", cmdutils.NpmConfigAuthTokenKey, dummyToken))
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
				assert.Contains(t, yarnrcContent, fmt.Sprintf("\"//acme.jfrog.io/artifactory/api/npm/test-repo:%s\" %s", cmdutils.NpmConfigAuthTokenKey, dummyToken))

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
		pipConfFilePath = filepath.Join(os.Getenv("APPDATA"), "pip", "pip.ini")
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

func TestSetupCommand_Go(t *testing.T) {
	goProxyEnv := "GOPROXY"
	// Restore the original value of the GOPROXY environment variable after the test.
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
		})
	}
}

// Test that configureGo unsets any existing GOPROXY env var before configuring.
func TestConfigureGo_UnsetEnv(t *testing.T) {
	testCmd := createTestSetupCommand(project.Go)
	// Simulate existing GOPROXY in environment
	t.Setenv("GOPROXY", "user:pass@dummy")
	// Ensure server details have credentials so configureGo proceeds
	testCmd.serverDetails.SetAccessToken(dummyToken)

	// Invoke configureGo directly
	require.NoError(t, testCmd.configureGo())
	// After calling, the GOPROXY env var should be cleared
	assert.Empty(t, os.Getenv("GOPROXY"), "GOPROXY should be unset by configureGo to avoid env override")
}

// Test that configureGo unsets any existing multi-entry GOPROXY env var before configuring.
func TestConfigureGo_UnsetEnv_MultiEntry(t *testing.T) {
    testCmd := createTestSetupCommand(project.Go)
    // Simulate existing multi-entry GOPROXY in environment
    t.Setenv("GOPROXY", "user:pass@dummy,goproxy2")
    // Ensure server details have credentials so configureGo proceeds
    testCmd.serverDetails.SetAccessToken(dummyToken)

    // Invoke configureGo directly
    require.NoError(t, testCmd.configureGo())
    // After calling, the GOPROXY env var should be cleared
    assert.Empty(t, os.Getenv("GOPROXY"), "GOPROXY should be unset by configureGo to avoid env override for multi-entry lists")
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

func TestSetupCommand_Maven(t *testing.T) {
	// Retrieve the home directory and construct the settings.xml file path.
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)
	settingsXml := filepath.Join(homeDir, ".m2", "settings.xml")

	// Back up the existing settings.xml file and ensure restoration after the test.
	restoreSettingsXml, err := ioutils.BackupFile(settingsXml, ".settings.xml.backup")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, restoreSettingsXml())
	}()

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
			settingsXmlContentBytes, err := os.ReadFile(settingsXml)
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
			assert.NoError(t, os.Remove(settingsXml))
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

			// Since the exact URL can vary (especially with extra paths),
			// just check that it contains the essential parts
			assert.Contains(t, pypircContent, "repository")
			assert.Contains(t, pypircContent, "https://acme.jfrog.io/artifactory/api")
			assert.Contains(t, pypircContent, "test-repo")

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
