package npm

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/gofrog/version"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	commonTests "github.com/jfrog/jfrog-cli-core/v2/common/tests"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
			"user-agent=npm/5.5.1 node/v8.9.1 darwin x64\n" +
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
			"user-agent=npm/5.5.1 node/v8.9.1 darwin x64",
			"@jfrog:registry = " + testRegistry,
			"email=ddd@dd.dd",
			"cache-lock-retries=10",
			"registry = " + testRegistry,
		}

	npmi := NpmCommand{registry: testRegistry, jsonOutput: true, npmAuth: "_auth = " + getTestCredentialValue(), npmVersion: version.NewVersion("9.5.0")}
	configAfter, err := npmi.prepareConfigData(configBefore)
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

	// Assert that NPM_CONFIG__AUTH environment variable was set
	assert.Equal(t, getTestCredentialValue(), os.Getenv(fmt.Sprintf(npmConfigAuthEnv, "//goodRegistry/", utils.NpmConfigAuthKey)))
	testsUtils.UnSetEnvAndAssert(t, fmt.Sprintf(npmConfigAuthEnv, "//goodRegistry/", utils.NpmConfigAuthKey))
}

func TestSetNpmConfigAuthEnv(t *testing.T) {
	testCases := []struct {
		name        string
		npmCm       *NpmCommand
		authKey     string
		value       string
		expectedEnv string
	}{
		{
			name: "set scoped registry auth env",
			npmCm: &NpmCommand{
				npmVersion: version.NewVersion("9.3.1"),
			},
			authKey:     utils.NpmConfigAuthKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config_//registry.example.com/:_auth",
		},
		{
			name: "set scoped registry authToken env",
			npmCm: &NpmCommand{
				npmVersion: version.NewVersion("9.3.1"),
			},
			authKey:     utils.NpmConfigAuthTokenKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config_//registry.example.com/:_authToken",
		},
		{
			name: "set legacy auth env",
			npmCm: &NpmCommand{
				npmVersion: version.NewVersion("8.16.3"),
			},
			authKey:     utils.NpmConfigAuthKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config__auth",
		},
		{
			name: "set legacy auth env even though authToken is passed",
			npmCm: &NpmCommand{
				npmVersion: version.NewVersion("8.16.3"),
			},
			authKey:     utils.NpmConfigAuthTokenKey,
			value:       "some_auth_token",
			expectedEnv: "npm_config__auth",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.npmCm.registry = "https://registry.example.com"
			err := tc.npmCm.setNpmConfigAuthEnv(tc.value, tc.authKey)
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

func TestParsePackageSpec(t *testing.T) {
	scope, name, ver, ok := parsePackageSpec("@angular/core@15.0.0")
	assert.True(t, ok)
	assert.Equal(t, "angular", scope)
	assert.Equal(t, "core", name)
	assert.Equal(t, "15.0.0", ver)

	scope, name, ver, ok = parsePackageSpec("lodash@4.17.21")
	assert.True(t, ok)
	assert.Equal(t, "", scope)
	assert.Equal(t, "lodash", name)
	assert.Equal(t, "4.17.21", ver)

	_, _, _, ok = parsePackageSpec("lodash")
	assert.False(t, ok)

	_, _, _, ok = parsePackageSpec("@angular@15.0.0")
	assert.False(t, ok)

	_, _, _, ok = parsePackageSpec("lodash@")
	assert.False(t, ok)
}

func TestHandle404Errors(t *testing.T) {
	nc := &NpmCommand{}

	err := nc.handle404Errors(errors.New("some random error"))
	assert.Nil(t, err)

	err = nc.handle404Errors(errors.New("No matching version found for lodash@4.17.21"))
	assert.Nil(t, err)
}

func TestBuildPackageTarballUrl(t *testing.T) {
	nc := &NpmCommand{}
	nc.SetRepo("npm-remote")

	// Scoped package
	url := nc.buildPackageTarballUrl("https://artifactory.example.com", "angular", "core", "15.0.0")
	assert.Equal(t, "https://artifactory.example.com/api/npm/npm-remote/@angular/core/-/core-15.0.0.tgz", url)

	// Regular package
	url = nc.buildPackageTarballUrl("https://artifactory.example.com", "", "lodash", "4.17.21")
	assert.Equal(t, "https://artifactory.example.com/api/npm/npm-remote/lodash/-/lodash-4.17.21.tgz", url)
}

func TestHandle404ErrorsDetects403BlockedPackage(t *testing.T) {
	expectedNotice := "package lodash:4.17.21 download was blocked by jfrog packages curation service due to the following policies violated {test-policy}"

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "7.137.0"}`))
			return
		}
		w.Header().Set("Npm-Notice", expectedNotice)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer testServer.Close()

	serverDetails := &config.ServerDetails{
		Url:            testServer.URL + "/",
		ArtifactoryUrl: testServer.URL + "/",
	}
	nc := &NpmCommand{}
	nc.SetRepo("npm-remote").SetServerDetails(serverDetails)

	err := nc.handle404Errors(errors.New("No matching version found for lodash@4.17.21"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403 Forbidden")
	assert.Contains(t, err.Error(), "lodash@4.17.21")
	assert.Contains(t, err.Error(), expectedNotice)
}

func TestHandle404ErrorsDetects403ScopedPackage(t *testing.T) {
	expectedNotice := "package @angular/core:15.0.0 download was blocked by jfrog packages curation service"

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "7.137.0"}`))
			return
		}
		w.Header().Set("Npm-Notice", expectedNotice)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer testServer.Close()

	serverDetails := &config.ServerDetails{
		Url:            testServer.URL + "/",
		ArtifactoryUrl: testServer.URL + "/",
	}
	nc := &NpmCommand{}
	nc.SetRepo("npm-remote").SetServerDetails(serverDetails)

	err := nc.handle404Errors(errors.New("No matching version found for @angular/core@15.0.0"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403 Forbidden")
	assert.Contains(t, err.Error(), "@angular/core@15.0.0")
	assert.Contains(t, err.Error(), expectedNotice)
}

func TestHandle404ErrorsNoBlockWhenServerReturns200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "7.137.0"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	serverDetails := &config.ServerDetails{
		Url:            testServer.URL + "/",
		ArtifactoryUrl: testServer.URL + "/",
	}
	nc := &NpmCommand{}
	nc.SetRepo("npm-remote").SetServerDetails(serverDetails)

	err := nc.handle404Errors(errors.New("No matching version found for lodash@4.17.21"))
	assert.NoError(t, err)
}

func TestHandle404ErrorsFallsBackToGetWhenNoNpmNoticeHeader(t *testing.T) {
	expectedBody := `{"error":"Package blocked by curation policy"}`

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/system/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "7.137.0"}`))
			return
		}
		// HEAD returns 403 without Npm-Notice; GET returns 403 with body
		w.WriteHeader(http.StatusForbidden)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(expectedBody))
		}
	}))
	defer testServer.Close()

	serverDetails := &config.ServerDetails{
		Url:            testServer.URL + "/",
		ArtifactoryUrl: testServer.URL + "/",
	}
	nc := &NpmCommand{}
	nc.SetRepo("npm-remote").SetServerDetails(serverDetails)

	err := nc.handle404Errors(errors.New("No matching version found for lodash@4.17.21"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403 Forbidden")
	assert.Contains(t, err.Error(), "lodash@4.17.21")
	assert.Contains(t, err.Error(), expectedBody)
}
