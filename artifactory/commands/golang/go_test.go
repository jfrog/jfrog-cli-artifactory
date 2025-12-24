package golang

import (
	"fmt"
	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	testsutils "github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPackageVersionRequest(t *testing.T) {
	tests := []struct {
		packageName     string
		branchName      string
		expectedRequest string
	}{
		{"github.com/jfrog/jfrog-cli", "", "github.com/jfrog/jfrog-cli/@v/latest.info"},
		{"github.com/jfrog/jfrog-cli", "dev", "github.com/jfrog/jfrog-cli/@v/dev.info"},
		{"github.com/jfrog/jfrog-cli", "v1.0.7", "github.com/jfrog/jfrog-cli/@v/v1.0.7.info"},
	}
	for _, test := range tests {
		t.Run(test.expectedRequest, func(t *testing.T) {
			versionRequest := buildPackageVersionRequest(test.packageName, test.branchName)
			if versionRequest != test.expectedRequest {
				t.Error("Failed to build package version request. The version request is", versionRequest, " but it is expected to be", test.expectedRequest)
			}
		})
	}
}

func TestGetPackageFilesPath(t *testing.T) {
	packageCachePath, err := biutils.GetGoModCachePath()
	assert.NoError(t, err)
	packageName := "github.com/golang/mock/mockgen"
	version := "v1.4.1"
	expectedPackagePath := filepath.Join(packageCachePath, "github.com/golang/mock@"+version)
	err = os.MkdirAll(expectedPackagePath, os.ModePerm)
	assert.NoError(t, err)
	defer testsutils.RemoveAllAndAssert(t, expectedPackagePath)
	actualPackagePath, err := getFileSystemPackagePath(packageCachePath, packageName, version)
	assert.NoError(t, err)
	assert.Equal(t, expectedPackagePath, actualPackagePath)
}

func TestSetArtifactoryAsResolutionServer(t *testing.T) {
	server := &config.ServerDetails{
		Url:            "http://localhost:8080/",
		ArtifactoryUrl: "http://localhost:8080/artifactory/",
		User:           "myUser",
		Password:       "myPassword",
		ServerId:       "myServer",
	}
	repo := "myRepo"

	// Setting the GOPROXY value to "" to ensure that the new value set in SetArtifactoryAsResolutionServer is correctly validated.
	cleanup := testsutils.SetEnvWithCallbackAndAssert(t, "GOPROXY", "")
	defer cleanup()

	assert.NoError(t, SetArtifactoryAsResolutionServer(server, repo, GoProxyUrlParams{Direct: true}))

	serverUrlWithoutHttp := strings.TrimPrefix(server.ArtifactoryUrl, "http://")
	expectedGoProxy := fmt.Sprintf("http://%s:%s@%sapi/go/%s|direct", server.User, server.Password, serverUrlWithoutHttp, repo)
	assert.Equal(t, expectedGoProxy, os.Getenv("GOPROXY"))

	// Verify that the EndpointPrefix value is correctly added to the GOPROXY.
	// In this test case, the endpoint prefix is set to api/curation/audit/.
	// This parameter allows downloading dependencies from a custom API instead of the default one.
	assert.NoError(t, SetArtifactoryAsResolutionServer(server, repo, GoProxyUrlParams{Direct: true, EndpointPrefix: coreutils.CurationPassThroughApi}))

	serverUrlWithoutHttp = strings.TrimPrefix(server.ArtifactoryUrl, "http://")
	expectedGoProxy = fmt.Sprintf("http://%s:%s@%sapi/curation/audit/api/go/%s|direct", server.User, server.Password, serverUrlWithoutHttp, repo)
	assert.Equal(t, expectedGoProxy, os.Getenv("GOPROXY"))
}

func TestGetArtifactoryRemoteRepoUrl(t *testing.T) {
	// testFakeToken is a fake test token for unit testing only - NOT a real secret
	// #nosec G101 -- This is a fake test token with no real credentials.
	testFakeToken := "fake-test-token-12345" //nolint:gosec
	server := &config.ServerDetails{
		ArtifactoryUrl: "https://server.com/artifactory",
		User:           "testuser",
		AccessToken:    testFakeToken,
	}
	repoName := "test-repo"
	repoUrl, err := GetArtifactoryRemoteRepoUrl(server, repoName, GoProxyUrlParams{})
	assert.NoError(t, err)
	assert.Equal(t, "https://testuser:"+testFakeToken+"@server.com/artifactory/api/go/test-repo", repoUrl)
}

func TestGetArtifactoryApiUrl(t *testing.T) {
	// testFakeToken is a fake test token for unit testing only - NOT a real secret
	// #nosec G101 -- This is a fake test token with no real credentials.
	testFakeToken := "fake-test-token-12345" //nolint:gosec

	details := auth.NewArtifactoryDetails()
	details.SetUrl("https://test.com/artifactory/")

	// Test username and password
	details.SetUser("frog")
	details.SetPassword("testpass")
	url, err := getArtifactoryApiUrl("test-repo", details, GoProxyUrlParams{})
	assert.NoError(t, err)
	assert.Equal(t, "https://frog:testpass@test.com/artifactory/api/go/test-repo", url)

	// Test username and password with EndpointPrefix and direct
	details.SetUser("frog")
	details.SetPassword("testpass")
	url, err = getArtifactoryApiUrl("test-repo", details, GoProxyUrlParams{EndpointPrefix: "test", Direct: true})
	assert.NoError(t, err)
	assert.Equal(t, "https://frog:testpass@test.com/artifactory/test/api/go/test-repo|direct", url)

	// Test access token
	// Set fake access token with username "test"
	details.SetUser("testuser")
	details.SetAccessToken(testFakeToken)
	url, err = getArtifactoryApiUrl("test-repo", details, GoProxyUrlParams{})
	assert.NoError(t, err)
	assert.Equal(t, "https://testuser:"+testFakeToken+"@test.com/artifactory/api/go/test-repo", url)

	// Test access token with username
	// Set fake access token with username "test"
	// Expect username to be "frog"
	details.SetUser("frog")
	details.SetAccessToken(testFakeToken)
	url, err = getArtifactoryApiUrl("test-repo", details, GoProxyUrlParams{})
	assert.NoError(t, err)
	assert.Equal(t, "https://frog:"+testFakeToken+"@test.com/artifactory/api/go/test-repo", url)
}

func TestGoProxyUrlParams_BuildUrl(t *testing.T) {
	testCases := []struct {
		name           string
		RepoName       string
		Direct         bool
		EndpointPrefix string
		ExpectedUrl    string
	}{
		{
			name:        "Url Without direct or Prefix",
			RepoName:    "go",
			ExpectedUrl: "https://test/api/go/go",
		},
		{
			name:        "Url With direct",
			RepoName:    "go",
			Direct:      true,
			ExpectedUrl: "https://test/api/go/go|direct",
		},
		{
			name:           "Url With Prefix",
			RepoName:       "go",
			EndpointPrefix: "prefix",
			ExpectedUrl:    "https://test/prefix/api/go/go",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			remoteUrl, err := url.Parse("https://test")
			require.NoError(t, err)
			gdu := &GoProxyUrlParams{
				Direct:         testCase.Direct,
				EndpointPrefix: testCase.EndpointPrefix,
			}
			assert.Equalf(t, testCase.ExpectedUrl, gdu.BuildUrl(remoteUrl, testCase.RepoName), "BuildUrl(%v, %v)", remoteUrl, testCase.RepoName)
		})
	}
}
