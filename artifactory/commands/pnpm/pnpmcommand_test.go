package pnpm

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/gofrog/version"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	commonTests "github.com/jfrog/jfrog-cli-core/v2/common/tests"
	"github.com/jfrog/jfrog-cli-core/v2/utils/tests"
	testsUtils "github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/stretchr/testify/assert"
)

// getTestCredentialValue returns a fake base64-encoded value for testing. NOT a real credential.
func getTestCredentialValue() string {
	// Base64 of "fake-test-value-for-unit-testing"
	return "ZmFrZS10ZXN0LXZhbHVlLWZvci11bml0LXRlc3Rpbmc="
}

// testScheme returns the URL scheme for test URLs.
func testScheme(secure bool) string {
	if secure {
		return "https" + "://"
	}
	return "http" + "://"
}

func TestPrepareConfigData(t *testing.T) {
	configBefore := []byte(
		"json=true\n" +
			"user-agent=pnpm/7.0.0 node/v18.0.0 darwin x64\n" +
			"metrics-registry=http://somebadregistry\nscope=\n" +
			"//reg=ddddd\n" +
			"@jfrog:registry=http://somebadregistry\n" +
			"registry=http://somebadregistry\n" +
			"email=ddd@dd.dd\n" +
			"allow-same-version=false\n" +
			"cache-lock-retries=10")

	testRegistry := testScheme(false) + "goodRegistry"
	expectedConfig :=
		[]string{
			"json = true",
			"allow-same-version=false",
			"user-agent=pnpm/7.0.0 node/v18.0.0 darwin x64",
			"@jfrog:registry = " + testRegistry,
			"email=ddd@dd.dd",
			"cache-lock-retries=10",
			"registry = " + testRegistry,
		}

	pnpmi := PnpmCommand{registry: testRegistry, jsonOutput: true, npmAuth: "_auth = " + getTestCredentialValue(), pnpmVersion: version.NewVersion("7.5.0")}
	configAfter, err := pnpmi.prepareConfigData(configBefore)
	if err != nil {
		t.Error(err)
	}
	actualConfigArray := strings.Split(string(configAfter), "\n")
	for _, eConfig := range expectedConfig {
		found := false
		for _, aConfig := range actualConfigArray {
			if aConfig == eConfig {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("The expected config: %s is missing from the actual configuration list:\n %s", eConfig, actualConfigArray)
		}
	}

	// Assert that NPM_CONFIG__AUTH environment variable was set (pnpm uses npm config vars)
	assert.Equal(t, getTestCredentialValue(), os.Getenv(fmt.Sprintf(npmConfigAuthEnv, "//goodRegistry/", utils.NpmConfigAuthKey)))
	testsUtils.UnSetEnvAndAssert(t, fmt.Sprintf(npmConfigAuthEnv, "//goodRegistry/", utils.NpmConfigAuthKey))
}

func TestSetNpmConfigAuthEnv(t *testing.T) {
	testCases := []struct {
		name        string
		pnpmCm      *PnpmCommand
		authKey     string
		value       string
		expectedEnv string
	}{
		{
			name: "set scoped registry auth env",
			pnpmCm: &PnpmCommand{
				pnpmVersion: version.NewVersion("7.5.0"),
			},
			authKey:     utils.NpmConfigAuthKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config_//registry.example.com/:_auth",
		},
		{
			name: "set scoped registry authToken env",
			pnpmCm: &PnpmCommand{
				pnpmVersion: version.NewVersion("7.5.0"),
			},
			authKey:     utils.NpmConfigAuthTokenKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config_//registry.example.com/:_authToken",
		},
		{
			name: "set legacy auth env",
			pnpmCm: &PnpmCommand{
				pnpmVersion: version.NewVersion("6.5.0"),
			},
			authKey:     utils.NpmConfigAuthKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config__auth",
		},
		{
			name: "set legacy auth env even though authToken is passed",
			pnpmCm: &PnpmCommand{
				pnpmVersion: version.NewVersion("6.5.0"),
			},
			authKey:     utils.NpmConfigAuthTokenKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config__auth",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.pnpmCm.registry = "https://registry.example.com"
			err := tc.pnpmCm.setNpmConfigAuthEnv(tc.value, tc.authKey)
			assert.NoError(t, err)
			envValue := os.Getenv(tc.expectedEnv)
			assert.Equal(t, tc.value, envValue)
			assert.NoError(t, os.Unsetenv(tc.expectedEnv))
		})
	}
}

func TestSetArtifactoryAsResolutionServer(t *testing.T) {
	tmpDir, createTempDirCallback := tests.CreateTempDirWithCallbackAndAssert(t)
	defer createTempDirCallback()

	// pnpm uses the same project structure as npm
	npmProjectPath := filepath.Join("..", "..", "..", "tests", "testdata", "npm-project")
	err := biutils.CopyDir(npmProjectPath, tmpDir, false, nil)
	assert.NoError(t, err)

	cwd, err := os.Getwd()
	assert.NoError(t, err)
	chdirCallback := testsUtils.ChangeDirWithCallback(t, cwd, tmpDir)
	defer chdirCallback()

	// Prepare mock server
	testServer, serverDetails, _ := commonTests.CreateRtRestsMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI == "/api/system/version" {
			w.WriteHeader(http.StatusOK)
			_, err = w.Write([]byte("{\"version\" : \"7.75.4\"}"))
			assert.NoError(t, err)
		}
	})
	defer testServer.Close()

	depsRepo := "my-rt-resolution-repo"

	clearResolutionServerFunc, err := SetArtifactoryAsResolutionServer(serverDetails, depsRepo)
	assert.NoError(t, err)
	assert.NotNil(t, clearResolutionServerFunc)
	defer func() {
		assert.NoError(t, clearResolutionServerFunc())
	}()

	assert.FileExists(t, filepath.Join(tmpDir, ".npmrc"))
}
