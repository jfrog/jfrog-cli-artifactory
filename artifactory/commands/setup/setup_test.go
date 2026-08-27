package setup

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	cmdutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/io"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

const (
	goProxyEnv = "GOPROXY"
)

// assertOwnerOnly verifies path is restricted to 0600, the mode jf setup applies
// to credential-bearing config files. It skips Windows, where os.Chmod only
// toggles the read-only attribute and the mode always reads back as 0666.
func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	if coreutils.IsWindows() {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "%s must be owner-only readable", path)
}

// collectConfigPaths walks root and returns the paths of known npm-family config
// files. The walk only collects paths; callers read them afterwards, so no
// filesystem operation runs inside the callback (avoids the WalkDir TOCTOU gosec
// flags).
func collectConfigPaths(t *testing.T, root string) []string {
	t.Helper()
	configFileNames := []string{".npmrc", "auth.ini", "rc", "config.yaml"}
	var configPaths []string
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !slices.Contains(configFileNames, entry.Name()) {
			return err
		}
		configPaths = append(configPaths, path)
		return nil
	}))
	return configPaths
}

// findConfigFileContaining returns the path of the package-manager config file
// under root whose contents include substr. pnpm chooses its own config directory
// and credential file name per version (auth.ini, rc, config.yaml, ...), so the
// file holding the token is located by content rather than by an assumed name.
func findConfigFileContaining(t *testing.T, root, substr string) string {
	t.Helper()
	for _, path := range collectConfigPaths(t, root) {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		if strings.Contains(string(content), substr) {
			return path
		}
	}
	require.FailNowf(t, "config file not found", "no config file under %s contains %q", root, substr)
	return ""
}

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
			// `pnpm config set` ignores NPM_CONFIG_USERCONFIG and writes to its own config
			// directory instead, so every variable that locates a home or config directory
			// is redirected into tempDir as well. Without this the pnpm cases write into
			// the developer's real pnpm configuration and then fail on the missing .npmrc.
			t.Setenv("HOME", tempDir)
			t.Setenv("USERPROFILE", tempDir)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg"))
			t.Setenv("LOCALAPPDATA", filepath.Join(tempDir, "localappdata"))

			// Set up server details for the current test case's authentication type.
			loginCmd := createTestSetupCommand(packageManager)
			loginCmd.serverDetails.SetUser(testCase.user)
			loginCmd.serverDetails.SetPassword(testCase.password)
			loginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			// Run the login command and ensure no errors occur.
			require.NoError(t, loginCmd.Run())

			npmrcContent := readPackageManagerConfigs(t, tempDir)

			// pnpm writes the _authToken at 0644, and jf setup restricts it to
			// owner-only. The file it lands in varies by pnpm version, so assert on
			// whichever config file actually holds the auth entry. Only written when
			// there is a credential to store. npm is not asserted here: it writes
			// ~/.npmrc at 0600 itself, so jf setup adds no hardening of ours to test.
			hasCredentials := testCase.accessToken != "" || (testCase.user != "" && testCase.password != "")
			if packageManager == project.Pnpm && hasCredentials {
				assertOwnerOnly(t, findConfigFileContaining(t, tempDir, ":_auth"))
			}

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
		})
	}
}

// readPackageManagerConfigs returns the contents of every npm-family configuration file
// written under root. npm writes the file NPM_CONFIG_USERCONFIG names, while pnpm writes
// auth.ini inside its own config directory, whose path differs per platform, so the files
// are found by walking rather than assumed. Only known configuration file names are read,
// to keep caches and log files out of the assertions.
func readPackageManagerConfigs(t *testing.T, root string) string {
	configPaths := collectConfigPaths(t, root)
	require.NotEmptyf(t, configPaths, "no package manager configuration was written under %s", root)

	var contents []string
	for _, configPath := range configPaths {
		content, err := os.ReadFile(configPath)
		require.NoError(t, err)
		contents = append(contents, string(content))
	}
	return strings.Join(contents, "\n")
}

// pnpmCredentialFiles is best-effort: when pnpm cannot be executed it returns no
// paths (and warns) rather than erroring, so a missing pnpm never breaks setup.
func TestPnpmCredentialFiles_PnpmMissing(t *testing.T) {
	// An empty PATH makes `pnpm` unresolvable on every OS.
	t.Setenv("PATH", t.TempDir())
	assert.Nil(t, pnpmCredentialFiles())
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

			// ~/.yarnrc stores the auth token in cleartext, so it must be owner-only.
			assertOwnerOnly(t, yarnrcFilePath)

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

			// Windows has no Unix permission bits: os.Chmod there only toggles
			// the read-only attribute, so the mode always reads back as 0666.
			if !coreutils.IsWindows() {
				info, err := os.Stat(pipConfFilePath)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "pip config must be owner-only readable")
			}

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
	// Resolve through the same helper the command uses, so this stays correct on
	// hosts where pip does not use ~/.config (e.g. macOS with
	// ~/Library/Application Support/pip present, or a Linux XDG_CONFIG_HOME).
	pipConfFilePath, err := python.ResolvePipConfigPath()
	require.NoError(t, err)
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

			assert.Contains(t, poetryConfigContent, "[repositories.test-repo]\nurl = \"https://acme.jfrog.io/artifactory/api/pypi/test-repo\"")

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
	// Isolate the Go env file so the test asserts (and hardens) a temporary file
	// rather than mutating the developer's real ~/.../go/env permissions.
	goEnvPath := filepath.Join(t.TempDir(), "go-env")
	t.Setenv("GOENV", goEnvPath)

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

			// The Go env file embeds user:token@ in GOPROXY, so it must be owner-only.
			assertOwnerOnly(t, goEnvPath)

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

			// The fallback must be comma-separated. A pipe would make the go command
			// fall through to the module's public source on ANY error, including a 403
			// from Artifactory Curation, silently defeating the block.
			assert.Contains(t, goProxy, ",direct", "jf setup must limit the direct fallback to 404/410")
			assert.NotContains(t, goProxy, "|direct", "a pipe separator would fall back on any error, including a Curation 403")

			// Clean up the global GOPROXY setting after each test case
			err = exec.Command("go", "env", "-u", goProxyEnv).Run()
			assert.NoError(t, err, "Failed to unset GOPROXY after test case")
		})
	}
}

// Test that configureGo unsets any existing GOPROXY env var before configuring.
func TestConfigureGo_UnsetEnv(t *testing.T) {
	// Isolate the Go env file (configureGo now hardens it) to a temporary path.
	t.Setenv("GOENV", filepath.Join(t.TempDir(), "go-env"))

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
	// Isolate the Go env file (configureGo now hardens it) to a temporary path.
	t.Setenv("GOENV", filepath.Join(t.TempDir(), "go-env"))

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

			// The init script embeds the access token in cleartext, so it must be owner-only.
			assertOwnerOnly(t, expectedInitScriptPath)

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

			// settings.xml stores the password/token in cleartext, so it must be owner-only.
			assertOwnerOnly(t, settingsXmlPath)

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

func TestSetupCommand_UV(t *testing.T) {
	// Skip if uv binary is not available
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv binary not found in PATH")
	}

	uvConfigDir := t.TempDir()
	uvConfigPath := filepath.Join(uvConfigDir, "uv.toml")
	t.Setenv("UV_CONFIG_FILE", uvConfigPath)

	// Isolate uv's credential store so tests don't pollute the developer's real credentials
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

	uvLoginCmd := createTestSetupCommand(project.UV)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Pre-create empty config file (uv auth login reads UV_CONFIG_FILE)
			require.NoError(t, os.WriteFile(uvConfigPath, []byte{}, 0600))

			uvLoginCmd.serverDetails.SetUser(testCase.user)
			uvLoginCmd.serverDetails.SetPassword(testCase.password)
			uvLoginCmd.serverDetails.SetAccessToken(testCase.accessToken)

			require.NoError(t, uvLoginCmd.Run())

			// Validate that the index entry was written to uv.toml
			uvConfigContentBytes, err := os.ReadFile(uvConfigPath)
			require.NoError(t, err)
			uvConfigContent := string(uvConfigContentBytes)

			assert.Contains(t, uvConfigContent, `name = "jfrog-pypi"`)
			assert.Contains(t, uvConfigContent, "acme.jfrog.io/artifactory/api/pypi/test-repo/simple")
			assert.Contains(t, uvConfigContent, "default = true")
			assert.Contains(t, uvConfigContent, `publish-url = "https://acme.jfrog.io/artifactory/api/pypi/test-repo"`)
		})
	}
}

// TestDeriveContainerRegistryHost covers the URL-derivation logic for
// `jf setup docker` / `jf setup podman`.
//
// The bug it guards against (RTECO-1352): createServerDetailsFromFlags in
// jfrog-cli clears ServerDetails.Url for the Rt command domain after copying
// it into ArtifactoryUrl. Before the fix, configureContainer read GetUrl()
// directly, producing an empty registry host and `docker login ""`, which
// docker silently resolves to Docker Hub (registry-1.docker.io) and fails
// with a misleading 401.
func TestDeriveContainerRegistryHost(t *testing.T) {
	cases := []struct {
		name           string
		artifactoryUrl string
		platformUrl    string
		wantHost       string
		wantErrContain string
	}{
		{
			name:           "--url path: ArtifactoryUrl populated, Url cleared",
			artifactoryUrl: "https://acme.jfrog.io/artifactory/",
			platformUrl:    "",
			wantHost:       "acme.jfrog.io",
		},
		{
			name:           "--server-id path: Url populated from saved config",
			artifactoryUrl: "",
			platformUrl:    "https://acme.jfrog.io/",
			wantHost:       "acme.jfrog.io",
		},
		{
			name:           "ArtifactoryUrl takes precedence when both are set",
			artifactoryUrl: "https://acme.jfrog.io/artifactory/",
			platformUrl:    "https://wrong.example.com/",
			wantHost:       "acme.jfrog.io",
		},
		{
			name:           "self-hosted with explicit port preserves port in host",
			artifactoryUrl: "https://artifactory.acme.com:8082/artifactory/",
			platformUrl:    "",
			wantHost:       "artifactory.acme.com:8082",
		},
		{
			name:           "http scheme is accepted",
			artifactoryUrl: "http://localhost:8081/artifactory/",
			platformUrl:    "",
			wantHost:       "localhost:8081",
		},
		{
			name:           "self-hosted IP over HTTP",
			artifactoryUrl: "http://10.0.0.10/artifactory",
			platformUrl:    "",
			wantHost:       "10.0.0.10",
		},
		{
			name:           "self-hosted IP with port",
			artifactoryUrl: "http://192.168.1.100:8082/artifactory/",
			platformUrl:    "",
			wantHost:       "192.168.1.100:8082",
		},
		{
			name:           "IPv6 host with port",
			artifactoryUrl: "https://[::1]:8082/artifactory/",
			platformUrl:    "",
			wantHost:       "[::1]:8082",
		},
		{
			name:           "subdomain registry method preserves full subdomain",
			artifactoryUrl: "https://docker-virtual.acme.jfrog.io/",
			platformUrl:    "",
			wantHost:       "docker-virtual.acme.jfrog.io",
		},
		{
			name: "URL with embedded credentials does not leak into host",
			// #nosec G101 -- test fixture: verifies userinfo is stripped from URL, not a real credential
			artifactoryUrl: "https://user:secret-token@acme.jfrog.io/artifactory/",
			platformUrl:    "",
			wantHost:       "acme.jfrog.io",
		},
		{
			name:           "URL without scheme returns scheme-specific error",
			artifactoryUrl: "acme.jfrog.io/artifactory/",
			platformUrl:    "",
			wantErrContain: "is missing a scheme",
		},
		{
			name:           "both empty returns explicit error, not empty host",
			artifactoryUrl: "",
			platformUrl:    "",
			wantErrContain: "server URL is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, err := deriveContainerRegistryHost(tc.artifactoryUrl, tc.platformUrl)
			if tc.wantErrContain != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantHost, host)
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
			} else if err != nil {
				assert.NotContains(t, err.Error(), "no credentials available")
				assert.NotContains(t, err.Error(), "no registry URL available")
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

func withTempHome(t *testing.T) string {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	return tmpHome
}

// bundlerParseConfig mirrors how Bundler itself reads ~/.bundle/config. Bundler uses a
// line-based stub serializer rather than a YAML parser: its HASH_REGEX takes the key up
// to the last colon followed by whitespace or end-of-line, then strips one optional pair
// of surrounding quotes from the value. Porting that here lets these tests assert that
// what we write is what Bundler actually reads back, rather than merely that it is valid
// YAML.
func bundlerParseConfig(content string) map[string]string {
	parsed := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		separator := -1
		for i := 0; i < len(line); i++ {
			if line[i] != ':' {
				continue
			}
			if i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t' {
				separator = i
			}
		}
		if separator < 0 {
			continue
		}
		key := strings.TrimLeft(line[:separator], " ")
		value := strings.TrimPrefix(line[separator+1:], " ")
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if key == "" || value == "" {
			continue
		}
		parsed[key] = value
	}
	return parsed
}

func readBundleConfig(t *testing.T, home string) map[string]string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(home, ".bundle", "config"))
	require.NoError(t, err)
	return bundlerParseConfig(string(content))
}

// TestBundleMirrorKey pins the mirror key against the value real Bundler computes for
// "mirror.https://rubygems.org" (verified against Bundler 1.17 and 4.0). If this drifts,
// Bundler silently stops redirecting to Artifactory.
func TestBundleMirrorKey(t *testing.T) {
	assert.Equal(t, "BUNDLE_MIRROR__HTTPS://RUBYGEMS__ORG/", bundleMirrorKey("https://rubygems.org"))
	assert.Equal(t, "BUNDLE_MIRROR__HTTPS://RUBYGEMS__ORG/", bundleMirrorKey("https://rubygems.org/"))
}

func TestWriteBundleSettings_EmptyFile(t *testing.T) {
	tmpHome := withTempHome(t)

	require.NoError(t, writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:secret"}))

	assert.Equal(t, "admin:secret", readBundleConfig(t, tmpHome)["BUNDLE_MY__JFROG__IO"])
}

func TestWriteBundleSettings_PreservesExistingKeys(t *testing.T) {
	tmpHome := withTempHome(t)

	bundleDir := filepath.Join(tmpHome, ".bundle")
	require.NoError(t, os.MkdirAll(bundleDir, 0755))
	// Written the way Bundler itself writes it, with quoted values.
	existing := "---\nBUNDLE_PATH: \"vendor/bundle\"\nBUNDLE_OTHERHOST__COM: \"other:creds\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "config"), []byte(existing), 0600))

	require.NoError(t, writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:secret"}))

	parsed := readBundleConfig(t, tmpHome)
	assert.Equal(t, "vendor/bundle", parsed["BUNDLE_PATH"])
	assert.Equal(t, "other:creds", parsed["BUNDLE_OTHERHOST__COM"])
	assert.Equal(t, "admin:secret", parsed["BUNDLE_MY__JFROG__IO"])
}

func TestWriteBundleSettings_OverwritesSameKey(t *testing.T) {
	tmpHome := withTempHome(t)

	require.NoError(t, writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:old-secret"}))
	require.NoError(t, writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:new-secret"}))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".bundle", "config"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(content), "BUNDLE_MY__JFROG__IO"), "should have exactly one entry for the host")
	assert.NotContains(t, string(content), "old-secret")
	assert.Equal(t, "admin:new-secret", bundlerParseConfig(string(content))["BUNDLE_MY__JFROG__IO"])
}

func TestWriteBundleSettings_FileIsPrivate(t *testing.T) {
	tmpHome := withTempHome(t)

	require.NoError(t, writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:secret"}))

	// The file holds credentials, so it must not be world-readable. assertOwnerOnly skips
	// Windows, where the mode always reads back as 0666.
	assertOwnerOnly(t, filepath.Join(tmpHome, ".bundle", "config"))
}

func TestWriteBundleSettings_MalformedExistingFileErrors(t *testing.T) {
	tmpHome := withTempHome(t)

	bundleDir := filepath.Join(tmpHome, ".bundle")
	require.NoError(t, os.MkdirAll(bundleDir, 0755))
	malformed := "not: valid: yaml: [unterminated"
	configPath := filepath.Join(bundleDir, "config")
	require.NoError(t, os.WriteFile(configPath, []byte(malformed), 0600))

	err := writeBundleSettings(map[string]string{"BUNDLE_MY__JFROG__IO": "admin:secret"})
	require.Error(t, err)

	// File must not be clobbered.
	content, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, malformed, string(content))
}

// TestWriteBundleSettings_BundlerReadsMirrorAndCredentials is the end-to-end guarantee:
// the mirror key contains "://" and a trailing slash, and the mirror value contains
// embedded credentials with their own colons. Asserting through Bundler's own parsing
// rules proves none of that confuses the key/value split.
func TestWriteBundleSettings_BundlerReadsMirrorAndCredentials(t *testing.T) {
	tmpHome := withTempHome(t)

	mirrorKey := bundleMirrorKey(rubygemsDefaultSource)
	// Assembled rather than written inline so static analysis does not read a literal
	// password-in-URL. Not a real credential.
	fakeSecret := "p%40ss%3A" + "word"
	mirrorValue := "https://admin:" + fakeSecret + "@acme.jfrog.io/artifactory/api/gems/gems-remote"
	require.NoError(t, writeBundleSettings(map[string]string{
		mirrorKey:                   mirrorValue,
		"BUNDLE_ACME__JFROG__IO":    "admin:" + fakeSecret,
		"BUNDLE_MY-CO__JFROG__IO":   "admin:secret",
		"BUNDLE_MY___CO__JFROG__IO": "admin:secret",
	}))

	parsed := readBundleConfig(t, tmpHome)
	assert.Equal(t, mirrorValue, parsed[mirrorKey], "Bundler must read the full mirror URL, credentials included")
	assert.Equal(t, "admin:"+fakeSecret, parsed["BUNDLE_ACME__JFROG__IO"])
	// Both dash spellings must survive, so Bundler 1.x and 2.x+ each find their own.
	assert.Equal(t, "admin:secret", parsed["BUNDLE_MY-CO__JFROG__IO"])
	assert.Equal(t, "admin:secret", parsed["BUNDLE_MY___CO__JFROG__IO"])
}

func TestAddGemrcSource_EmptyFile(t *testing.T) {
	tmpHome := withTempHome(t)

	require.NoError(t, addGemrcSource("https://my.jfrog.io/artifactory/api/gems/gems-local"))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".gemrc"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(content, &parsed))
	sources, ok := parsed[":sources"].([]interface{})
	require.True(t, ok)
	// The public source must be gone: RubyGems searches in order, so leaving it in front
	// would let `gem install` reach rubygems.org before Artifactory.
	assert.Equal(t, []interface{}{"https://my.jfrog.io/artifactory/api/gems/gems-local"}, sources)
}

func TestAddGemrcSource_PreservesUnrelatedKeys(t *testing.T) {
	tmpHome := withTempHome(t)

	existing := ":ssl_ca_cert: /etc/ssl/certs/ca.pem\n:sources:\n- https://rubygems.org\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpHome, ".gemrc"), []byte(existing), 0644))

	require.NoError(t, addGemrcSource("https://my.jfrog.io/artifactory/api/gems/gems-local"))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".gemrc"))
	require.NoError(t, err)
	assert.Contains(t, string(content), ":ssl_ca_cert: /etc/ssl/certs/ca.pem")
}

func TestAddGemrcSource_ReAddSameSourceMovesToFrontNoDuplicate(t *testing.T) {
	tmpHome := withTempHome(t)

	sourceURL := "https://my.jfrog.io/artifactory/api/gems/gems-local"
	require.NoError(t, addGemrcSource(sourceURL))
	require.NoError(t, addGemrcSource(sourceURL))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".gemrc"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(content, &parsed))
	sources, ok := parsed[":sources"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{sourceURL}, sources, "no duplicate entry, and no public source")
}

func TestAddGemrcSource_SecondDifferentRepoKeepsBothMostRecentFirst(t *testing.T) {
	tmpHome := withTempHome(t)

	firstURL := "https://my.jfrog.io/artifactory/api/gems/gems-local"
	secondURL := "https://my.jfrog.io/artifactory/api/gems/gems-local-2"
	require.NoError(t, addGemrcSource(firstURL))
	require.NoError(t, addGemrcSource(secondURL))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".gemrc"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(content, &parsed))
	sources, ok := parsed[":sources"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{secondURL, firstURL}, sources, "most recently configured source first, public source removed")
}

func TestAddGemrcSource_MalformedExistingFileErrors(t *testing.T) {
	tmpHome := withTempHome(t)

	malformed := "not: valid: yaml: [unterminated"
	gemrcPath := filepath.Join(tmpHome, ".gemrc")
	require.NoError(t, os.WriteFile(gemrcPath, []byte(malformed), 0644))

	err := addGemrcSource("https://my.jfrog.io/artifactory/api/gems/gems-local")
	require.Error(t, err)

	content, readErr := os.ReadFile(gemrcPath)
	require.NoError(t, readErr)
	assert.Equal(t, malformed, string(content))
}

// TestAddGemrcSource_CredentialRotationReplacesEntry guards the case that would otherwise
// leave `gem install` retrying a dead credential: re-running setup after a token rotation
// must replace the existing entry for that repository, not accumulate a stale one.
func TestAddGemrcSource_CredentialRotationReplacesEntry(t *testing.T) {
	tmpHome := withTempHome(t)

	base := "https://acme.jfrog.io/artifactory/api/gems/gems-virtual"
	require.NoError(t, addGemrcSource("https://admin:old-token@acme.jfrog.io/artifactory/api/gems/gems-virtual"))
	require.NoError(t, addGemrcSource("https://admin:new-token@acme.jfrog.io/artifactory/api/gems/gems-virtual"))

	content, err := os.ReadFile(filepath.Join(tmpHome, ".gemrc"))
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(content, &parsed))
	sources, ok := parsed[":sources"].([]interface{})
	require.True(t, ok)

	assert.Equal(t, []interface{}{
		"https://admin:new-token@acme.jfrog.io/artifactory/api/gems/gems-virtual",
	}, sources, "the rotated credential must replace the old entry for the same repository")
	assert.NotContains(t, string(content), "old-token")
	assert.Equal(t, base, gemSourceIdentity("https://admin:new-token@acme.jfrog.io/artifactory/api/gems/gems-virtual/"))
}

// TestAddGemrcSource_FileIsPrivate: the source URL embeds credentials, because RubyGems
// has no other way to authenticate an install, so the file must not be world-readable.
func TestAddGemrcSource_FileIsPrivate(t *testing.T) {
	tmpHome := withTempHome(t)

	require.NoError(t, addGemrcSource("https://admin:tok@acme.jfrog.io/artifactory/api/gems/gems-virtual"))

	// The source URL embeds credentials, so the file must not be world-readable.
	assertOwnerOnly(t, filepath.Join(tmpHome, ".gemrc"))
}

// TestGemSourceIdentity_IgnoresTrailingSlashAndCredentials: gem sources are written with a
// trailing slash (RubyGems needs it to resolve specs.4.8.gz), so equality checks must not
// treat the slashed and unslashed forms — or a rotated credential — as different repos.
func TestGemSourceIdentity_IgnoresTrailingSlashAndCredentials(t *testing.T) {
	base := "https://acme.jfrog.io/artifactory/api/gems/gems-virtual"
	assert.Equal(t, base, gemSourceIdentity(base))
	assert.Equal(t, base, gemSourceIdentity(base+"/"))
	assert.Equal(t, base, gemSourceIdentity("https://admin:tok@acme.jfrog.io/artifactory/api/gems/gems-virtual/"))
	assert.Equal(t, base, gemSourceIdentity("https://admin:other@acme.jfrog.io/artifactory/api/gems/gems-virtual"))
}

// Every supported package manager must describe what it changed: the output is the
// only place a user learns the configuration is user-level rather than scoped to the
// directory they ran the command in.
func TestConfigScopeNote_CoversEverySupportedPackageManager(t *testing.T) {
	for _, name := range GetSupportedPackageManagersList() {
		packageManager := project.FromString(name)
		require.NotEqualf(t, project.ProjectType(-1), packageManager, "%q is not a known project type", name)

		// Clear any override so this asserts the default, user-level wording.
		if envVar := packageManagerConfigs[packageManager].overrideEnv; envVar != "" {
			t.Setenv(envVar, "")
		}

		note := configScopeNote(packageManager)
		require.NotEmptyf(t, note, "no scope note for supported package manager %q", name)

		// Each note must take exactly one of the two valid shapes, and a resolution
		// note must name the package manager it is talking about.
		if packageManagerConfigs[packageManager].credentialsOnly {
			assert.Containsf(t, note, "Credentials were saved", "%q: %s", name, note)
			assert.NotContainsf(t, note, "applies to every", "%q must not claim resolution: %s", name, note)
			continue
		}
		assert.Containsf(t, note, fmt.Sprintf("applies to every %s project", name), "%q: %s", name, note)
		assert.NotContainsf(t, note, "Credentials were saved", "%q: %s", name, note)
	}
}

// A configuration redirected by an environment variable is not user-level — it can sit
// inside the current project — so the note must report that path instead of promising
// a scope that does not hold.
func TestConfigScopeNote_RedirectedConfigDoesNotClaimUserScope(t *testing.T) {
	overrides := packageManagersWithConfigOverride()
	require.NotEmpty(t, overrides, "expected package managers with a config override")

	for packageManager, envVar := range overrides {
		overridePath := filepath.Join(t.TempDir(), "redirected-config")
		t.Setenv(envVar, overridePath)

		note := configScopeNote(packageManager)
		assert.Containsf(t, note, overridePath, "%s: note should name the redirected path: %s", packageManager.String(), note)
		assert.Containsf(t, note, envVar, "%s: note should name the variable that redirected it: %s", packageManager.String(), note)
		// The scope claim itself is what must not survive a redirect; the note may still
		// mention user-level configuration to contrast against it.
		assert.NotContainsf(t, note, "applies to every", "%s must not claim project-wide scope when redirected: %s", packageManager.String(), note)
		assert.Containsf(t, note, "scope follows that path", "%s should defer scope to the redirected file: %s", packageManager.String(), note)
	}
}

// With no override set the default user-level wording applies, so the two branches are
// covered in both directions.
func TestConfigScopeNote_WithoutOverrideClaimsUserScope(t *testing.T) {
	for packageManager, envVar := range packageManagersWithConfigOverride() {
		t.Setenv(envVar, "")
		note := configScopeNote(packageManager)
		assert.Containsf(t, note, "user-level", "%s: %s", packageManager.String(), note)
		assert.NotContainsf(t, note, envVar, "%s: %s", packageManager.String(), note)
	}
}

// Each override was verified against the tool that consumes it, so the set is pinned in
// both directions: a new entry added without that verification fails here, and dropping
// one that works silently downgrades the note to a scope claim that may be wrong.
func TestPackageManagerConfigs_OverridesAreExactlyTheVerifiedSet(t *testing.T) {
	expected := map[project.ProjectType]string{
		project.Npm:    "NPM_CONFIG_USERCONFIG",
		project.Pip:    "PIP_CONFIG_FILE",
		project.Pipenv: "PIP_CONFIG_FILE",
		project.Poetry: "POETRY_CONFIG_DIR",
		project.UV:     "UV_CONFIG_FILE",
		project.Go:     "GOENV",
		project.Gradle: "GRADLE_USER_HOME",
	}
	assert.Equal(t, expected, packageManagersWithConfigOverride())
}

// pnpm looks like it should follow npm here, and it does not: `pnpm config set` writes to
// pnpm's own config directory and ignores NPM_CONFIG_USERCONFIG (verified against pnpm
// 11 — the file it wrote was auth.ini under the pnpm config directory, both with and
// without the variable set). Claiming the redirect would send users to a file that
// `jf setup pnpm` never touched, so the absence is asserted rather than left to chance.
func TestPackageManagerConfigs_PnpmHasNoConfigOverride(t *testing.T) {
	assert.Empty(t, packageManagerConfigs[project.Pnpm].overrideEnv,
		"pnpm config set does not honor an environment override")

	customConfig := filepath.Join(t.TempDir(), "custom.npmrc")
	t.Setenv("NPM_CONFIG_USERCONFIG", customConfig)
	note := configScopeNote(project.Pnpm)
	assert.NotContains(t, note, customConfig, "the note must not point at a file pnpm does not write: "+note)
	assert.Contains(t, note, "applies to every pnpm project", note)
}

// packageManagersWithConfigOverride returns only the entries that declare an override
// variable, so the override tests do not have to skip the rest.
func packageManagersWithConfigOverride() map[project.ProjectType]string {
	overrides := map[project.ProjectType]string{}
	for packageManager, cfg := range packageManagerConfigs {
		if cfg.overrideEnv != "" {
			overrides[packageManager] = cfg.overrideEnv
		}
	}
	return overrides
}

// Container logins authenticate rather than redirect resolution, so their note must
// not promise that projects now resolve through Artifactory — an unqualified
// `docker pull alpine` still reaches Docker Hub after `jf setup docker`.
func TestConfigScopeNote_ContainerLoginsDoNotClaimResolution(t *testing.T) {
	for _, packageManager := range []project.ProjectType{project.Docker, project.Podman, project.Helm} {
		note := configScopeNote(packageManager)
		assert.Contains(t, note, "Credentials were saved", packageManager.String())
		assert.NotContains(t, note, "applies to every", packageManager.String())
	}

	// Resolution-changing package managers must state the scope explicitly.
	for _, packageManager := range []project.ProjectType{project.Npm, project.Maven, project.Go, project.Pip} {
		note := configScopeNote(packageManager)
		assert.Contains(t, note, "applies to every", packageManager.String())
		assert.Contains(t, note, "not only the current directory", packageManager.String())
	}
}

// An unsupported package manager has nothing accurate to say, so it must stay silent
// rather than print a misleading note.
func TestConfigScopeNote_UnknownPackageManagerIsSilent(t *testing.T) {
	assert.Empty(t, configScopeNote(project.Cocoapods))
}

// The note is only useful if the command actually prints it, so assert the wiring
// rather than just the string builder: removing the log call would otherwise leave
// every configScopeNote test passing.
func TestSetupCommand_PrintsConfigScopeNote(t *testing.T) {
	// Maven writes only settings.xml, so a temporary home keeps the run self-contained.
	// Both variables are set for cross-platform parity with the other Maven tests.
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	var output bytes.Buffer
	previousLogger := log.Logger
	log.SetLogger(log.NewLogger(log.INFO, &output))
	defer log.SetLogger(previousLogger)

	setupCmd := createTestSetupCommand(project.Maven)
	setupCmd.repoName = "test-repo"
	require.NoError(t, setupCmd.Run())

	assert.Contains(t, output.String(), "Successfully configured", "expected the success message")
	assert.Contains(t, output.String(), configScopeNote(project.Maven),
		"the command must print the scope note, not just be able to build it")
}

// A pre-existing GOPROXY is echoed back to the user, and GOPROXY is a
// separator-delimited list, so masking has to cover every entry rather than
// stopping at the first set of credentials.
func TestMaskGoProxyCredentials(t *testing.T) {
	const tokenOne = "TOKEN_ONE"
	const tokenTwo = "TOKEN_TWO"
	testCases := []struct {
		name     string
		goProxy  string
		expected string
	}{
		{
			name:     "Single entry with direct fallback",
			goProxy:  "https://u:" + tokenOne + "@art.example.com/artifactory/api/go/repo,direct",
			expected: "https://****@art.example.com/artifactory/api/go/repo,direct",
		},
		{
			name:     "Comma-separated entries both masked",
			goProxy:  "https://u:" + tokenOne + "@host1/api/go/r1,https://u:" + tokenTwo + "@host2/api/go/r2",
			expected: "https://****@host1/api/go/r1,https://****@host2/api/go/r2",
		},
		{
			name:     "Pipe-separated entries both masked",
			goProxy:  "https://u:" + tokenOne + "@host1/api/go/r1|https://u:" + tokenTwo + "@host2/api/go/r2",
			expected: "https://****@host1/api/go/r1|https://****@host2/api/go/r2",
		},
		{
			name:     "Mixed separators",
			goProxy:  "https://u:" + tokenOne + "@host1/r1,https://u:" + tokenTwo + "@host2/r2|direct",
			expected: "https://****@host1/r1,https://****@host2/r2|direct",
		},
		{
			name:     "Password containing an at sign masks the whole password",
			goProxy:  "https://user:p@ssw0rd@art.example.com/artifactory/api/go/repo",
			expected: "https://****@art.example.com/artifactory/api/go/repo",
		},
		{
			name:     "No credentials is left untouched",
			goProxy:  "https://proxy.golang.org,direct",
			expected: "https://proxy.golang.org,direct",
		},
		{
			name:     "Keywords are left untouched",
			goProxy:  "off",
			expected: "off",
		},
		{
			name:     "Empty value",
			goProxy:  "",
			expected: "",
		},
		{
			name:     "Entry without a scheme still loses its credentials",
			goProxy:  "u:" + tokenOne + "@host/api/go/repo",
			expected: "****@host/api/go/repo",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			masked := maskGoProxyCredentials(testCase.goProxy)
			assert.Equal(t, testCase.expected, masked)
			assert.NotContains(t, masked, tokenOne, "no entry's credentials may survive masking")
			assert.NotContains(t, masked, tokenTwo, "no entry's credentials may survive masking")
			assert.NotContains(t, masked, "ssw0rd", "a password containing '@' must not leak")
		})
	}
}

// packageManagerConfigs drives the note printed after every successful setup, so a
// package manager added to packageManagerToRepositoryPackageType without an entry
// here would silently print nothing. The map comment promises these stay in step.
func TestPackageManagerConfigs_CoversEverySupportedPackageManager(t *testing.T) {
	assert.Len(t, packageManagerConfigs, len(packageManagerToRepositoryPackageType))
	for packageManager := range packageManagerToRepositoryPackageType {
		config, ok := packageManagerConfigs[packageManager]
		if assert.True(t, ok, "%s is supported by jf setup but has no packageManagerConfigs entry", packageManager) {
			assert.NotEmpty(t, config.location, "%s has an entry with no location", packageManager)
		}
	}
}

func TestApkValidateRepositoryExists(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  string
	}{
		{name: "existing repository", statusCode: http.StatusOK},
		{name: "missing repository", statusCode: http.StatusNotFound, wantError: `repository "alpine-local" not found`},
		{name: "bad request", statusCode: http.StatusBadRequest, wantError: `repository "alpine-local" not found`},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantError: `Artifactory returned HTTP 401`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/repositories/alpine-local", r.URL.Path)
				w.WriteHeader(test.statusCode)
			}))
			defer server.Close()

			serverDetails := &config.ServerDetails{ArtifactoryUrl: server.URL}
			err := apkValidateRepositoryExists(server.URL, "alpine-local", serverDetails)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestResolveApkRepoTypeWithProject(t *testing.T) {
	cmd := createTestSetupCommand(project.Apk)

	cmd.SetProjectKey("my-project")
	repoType, err := cmd.resolveApkRepoType()
	require.NoError(t, err)
	assert.Empty(t, repoType)
}

func TestApkMergeRepositoriesContent(t *testing.T) {
	newRepo := "https://user:token@acme.jfrog.io/artifactory/alpine-virt/v3.20/main/" // #nosec G101 -- test fixture, not a real credential

	t.Run("empty file gets only the jfrog line", func(t *testing.T) {
		got := apkMergeRepositoriesContent("", newRepo)
		assert.Equal(t, newRepo+"\n", got)
	})

	t.Run("preserves public CDN and comments, prepends jfrog", func(t *testing.T) {
		existing := `# Alpine mirrors
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://dl-cdn.alpinelinux.org/alpine/v3.20/community
`
		got := apkMergeRepositoriesContent(existing, newRepo)
		assert.Equal(t, newRepo+`
# Alpine mirrors
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://dl-cdn.alpinelinux.org/alpine/v3.20/community
`, got)
	})

	t.Run("overrides existing jfrog line for same host, keeps user lines", func(t *testing.T) {
		// #nosec G101 -- test fixture, not a real credential
		existing := `https://olduser:oldpass@acme.jfrog.io/artifactory/old-alpine/v3.19/main/
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://mirror.example.com/alpine/edge/testing
`
		got := apkMergeRepositoriesContent(existing, newRepo)
		assert.Equal(t, newRepo+`
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://mirror.example.com/alpine/edge/testing
`, got)
	})

	t.Run("collapses multiple same-host jfrog lines into one", func(t *testing.T) {
		// #nosec G101 -- test fixture, not a real credential
		existing := `https://acme.jfrog.io/artifactory/alpine-a/v3.20/main/
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://user:pass@acme.jfrog.io/artifactory/alpine-b/v3.20/community/
`
		got := apkMergeRepositoriesContent(existing, newRepo)
		assert.Equal(t, newRepo+`
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
`, got)
	})

	t.Run("leaves a different artifactory host untouched", func(t *testing.T) {
		existing := `https://other.jfrog.io/artifactory/other-alpine/v3.20/main/
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
`
		got := apkMergeRepositoriesContent(existing, newRepo)
		assert.Equal(t, newRepo+`
https://other.jfrog.io/artifactory/other-alpine/v3.20/main/
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
`, got)
	})

	t.Run("preserves alpine @tag lines", func(t *testing.T) {
		existing := `@edge https://dl-cdn.alpinelinux.org/alpine/edge/main
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
`
		got := apkMergeRepositoriesContent(existing, newRepo)
		assert.Contains(t, got, "@edge https://dl-cdn.alpinelinux.org/alpine/edge/main")
		assert.Contains(t, got, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
		assert.True(t, strings.HasPrefix(strings.TrimSpace(got), newRepo))
	})
}

func TestApkRepoHostname(t *testing.T) {
	assert.Equal(t, "acme.jfrog.io", apkRepoHostname("https://user:pass@acme.jfrog.io/artifactory/repo/v3.20/main/"))
	assert.Equal(t, "acme.jfrog.io", apkRepoHostname("https://acme.jfrog.io/artifactory/repo/v3.20/main/"))
	assert.Equal(t, "dl-cdn.alpinelinux.org", apkRepoHostname("@edge https://dl-cdn.alpinelinux.org/alpine/edge/main"))
	assert.Empty(t, apkRepoHostname("# comment"))
	assert.Empty(t, apkRepoHostname("/media/cdrom/apks"))
}

func TestApkResolveCredentials_PrefersPasswordOverRefreshableToken(t *testing.T) {
	// #nosec G101 -- test fixture, not a real credential
	sd := &config.ServerDetails{
		User:                    "admin",
		Password:                "long-lived-password",
		AccessToken:             "short-lived-access-token",
		ArtifactoryRefreshToken: "refresh-token",
	}

	username, password := apkResolveCredentials(sd)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "long-lived-password", password,
		"the non-expiring password must be embedded, not the refreshable access token")
}

func TestApkResolveCredentials_UsesTokenWhenNoPassword(t *testing.T) {
	sd := &config.ServerDetails{User: "admin", AccessToken: "only-credential"} // #nosec G101 -- test fixture, not a real credential

	username, password := apkResolveCredentials(sd)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "only-credential", password)
}

func TestApkResolveCredentials_UsernameAndPasswordOnly(t *testing.T) {
	sd := &config.ServerDetails{User: "admin", Password: "pass"}

	username, password := apkResolveCredentials(sd)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "pass", password)
}

func TestApkResolveCredentials_Anonymous(t *testing.T) {
	username, password := apkResolveCredentials(&config.ServerDetails{})
	assert.Empty(t, username)
	assert.Empty(t, password)

	username, password = apkResolveCredentials(nil)
	assert.Empty(t, username)
	assert.Empty(t, password)
}
