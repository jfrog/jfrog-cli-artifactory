package yarn

import (
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestValidateSupportedCommand(t *testing.T) {
	yarnCmd := NewYarnCommand()

	testCases := []struct {
		args  []string
		valid bool
	}{
		{[]string{}, true},
		{[]string{"--json"}, true},
		{[]string{"npm", "publish", "--json"}, false},
		{[]string{"npm", "--json", "publish"}, false},
		{[]string{"npm", "tag", "list"}, false},
		{[]string{"npm", "info", "package-name"}, true},
		{[]string{"npm", "whoami"}, true},
		{[]string{"--version"}, true},
		{[]string{"set", "version", "4.0.1"}, true},
		{[]string{"set", "version", "3.2.1"}, true},
		// Yarn v5+ is not yet supported
		{[]string{"set", "version", "5.0.0"}, false},
	}

	for _, testCase := range testCases {
		yarnCmd.yarnArgs = testCase.args
		err := yarnCmd.validateSupportedCommand()
		assert.Equal(t, testCase.valid, err == nil, "Test args:", testCase.args)
	}
}

func TestSetAndRestoreEnvironmentVariables(t *testing.T) {
	const jfrogCliTestingEnvVar = "JFROG_CLI_ENV_VAR_FOR_TESTING"
	// Check backup and restore of an existing variable
	setEnvCallback := tests.SetEnvWithCallbackAndAssert(t, jfrogCliTestingEnvVar, "abc")
	backupEnvsMap := make(map[string]*string)
	oldVal, err := backupAndSetEnvironmentVariable(jfrogCliTestingEnvVar, "new-value")
	assert.NoError(t, err)
	assert.Equal(t, "new-value", os.Getenv(jfrogCliTestingEnvVar))
	backupEnvsMap[jfrogCliTestingEnvVar] = &oldVal
	assert.NoError(t, restoreEnvironmentVariables(backupEnvsMap))
	assert.Equal(t, "abc", os.Getenv(jfrogCliTestingEnvVar))

	// Check backup and restore of a variable that doesn't exist
	setEnvCallback()
	oldVal, err = backupAndSetEnvironmentVariable(jfrogCliTestingEnvVar, "another-value")
	assert.NoError(t, err)
	assert.Equal(t, "another-value", os.Getenv(jfrogCliTestingEnvVar))
	backupEnvsMap[jfrogCliTestingEnvVar] = &oldVal
	err = restoreEnvironmentVariables(backupEnvsMap)
	assert.NoError(t, err)
	_, exist := os.LookupEnv(jfrogCliTestingEnvVar)
	assert.False(t, exist)
}

func TestExtractAuthValuesFromNpmAuth(t *testing.T) {
	testCases := []struct {
		responseFromArtifactory     string
		expectedExtractedAuthIndent string
		expectedExtractedAuthToken  string
	}{
		{"_auth = Z290Y2hhISB5b3UgcmVhbGx5IHRoaW5rIGkgd291bGQgcHV0IHJlYWwgY3JlZGVudGlhbHMgaGVyZT8=\nalways-auth = true\nemail = notexist@mail.com\n", "Z290Y2hhISB5b3UgcmVhbGx5IHRoaW5rIGkgd291bGQgcHV0IHJlYWwgY3JlZGVudGlhbHMgaGVyZT8=", ""},
		{"always-auth=true\nemail=notexist@mail.com\n_auth=TGVhcCBhbmQgdGhlIHJlc3Qgd2lsbCBmb2xsb3c=\n", "TGVhcCBhbmQgdGhlIHJlc3Qgd2lsbCBmb2xsb3c=", ""},
		{"_authToken = ThisIsNotARealToken\nalways-auth = true\nemail = notexist@mail.com\n", "", "ThisIsNotARealToken"},
	}

	for _, testCase := range testCases {
		actualExtractedAuthIndent, actualExtractedAuthToken, err := extractAuthValFromNpmAuth(testCase.responseFromArtifactory)
		assert.NoError(t, err)
		assert.Equal(t, testCase.expectedExtractedAuthIndent, actualExtractedAuthIndent)
		assert.Equal(t, testCase.expectedExtractedAuthToken, actualExtractedAuthToken)
	}
}

func TestSkipVersionCheck(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"set version", []string{"set", "version", "1.22.10"}, true},
		{"long --version flag", []string{"--version"}, true},
		{"short -v flag", []string{"-v"}, true},
		{"install", []string{"install"}, false},
		{"add lodash", []string{"add", "lodash"}, false},
		{"set without version keyword", []string{"set", "resolution"}, false},
		{"set version missing value", []string{"set", "version"}, true},
		{"empty args", []string{}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := skipVersionCheck(testCase.args)
			assert.Equal(t, testCase.expected, result, "Test args:", testCase.args)
		})
	}
}

func TestValidateSetVersion(t *testing.T) {
	testCases := []struct {
		name       string
		setVersion string
		expectErr  bool
	}{
		{"yarn v1 classic", "1.22.22", false},
		{"yarn v3", "3.6.4", false},
		{"yarn v4 latest supported major", "4.0.1", false},
		{"yarn v4 high patch", "4.9.9", false},
		{"yarn v5 unsupported", "5.0.0", true},
		{"yarn v6 unsupported", "6.1.2", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateSetVersion(testCase.setVersion)
			if testCase.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestYarnCommandSetters(t *testing.T) {
	yc := NewYarnCommand()

	// SetArgs
	args := []string{"install", "--json"}
	yc.SetArgs(args)
	assert.Equal(t, args, yc.yarnArgs)

	// SetConfigFilePath
	yc.SetConfigFilePath("/tmp/config.yaml")
	assert.Equal(t, "/tmp/config.yaml", yc.configFilePath)

	// SetUseNative
	assert.False(t, yc.useNative)
	yc.SetUseNative(true)
	assert.True(t, yc.useNative)

	// SetBuildConfiguration
	bc := buildUtils.NewBuildConfiguration("my-build", "42", "my-module", "my-project")
	yc.SetBuildConfiguration(bc)
	assert.Equal(t, bc, yc.buildConfiguration)

	// SetServerDetails
	sd := &config.ServerDetails{ServerId: "my-server", Url: "https://example.com"}
	yc.SetServerDetails(sd)
	assert.Equal(t, sd, yc.serverDetails)

	// ServerDetails getter
	got, err := yc.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, sd, got)

	// CommandName
	assert.Equal(t, "rt_yarn", yc.CommandName())
}

func TestPrintMissingDependencies(t *testing.T) {
	// Should not panic on empty or non-empty input.
	assert.NotPanics(t, func() { printMissingDependencies(nil) })
	assert.NotPanics(t, func() { printMissingDependencies([]string{}) })
	assert.NotPanics(t, func() { printMissingDependencies([]string{"lodash:4.17.21", "chalk:5.3.0"}) })
}
