package python

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/tests"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRepoToPyprojectFile(t *testing.T) {
	poetryProjectPath, cleanUp := initPoetryTest(t)
	defer cleanUp()
	pyProjectPath := filepath.Join(poetryProjectPath, "pyproject.toml")
	dummyRepoName := "test-repo-name"
	dummyRepoURL := "https://ecosysjfrog.jfrog.io/"

	err := addRepoToPyprojectFile(pyProjectPath, dummyRepoName, dummyRepoURL)
	assert.NoError(t, err)
	// Validate pyproject.toml file content
	content, err := fileutils.ReadFile(pyProjectPath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), dummyRepoURL)
}

func initPoetryTest(t *testing.T) (string, func()) {
	// Create and change directory to test workspace
	testAbs, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "testdata", "poetry-project"))
	assert.NoError(t, err)
	poetryProjectPath, cleanUp := tests.CreateTestWorkspace(t, testAbs)
	return poetryProjectPath, cleanUp
}

// TestSetPypiRepoUrlWithCredentials_URLTransformation verifies that the publish URL drops the
// /simple suffix while every resolution command keeps it, so Poetry queries a PEP 503 index.
func TestSetPypiRepoUrlWithCredentials_URLTransformation(t *testing.T) {
	tests := []struct {
		name        string
		repository  string
		serverURL   string
		username    string
		password    string
		accessToken string
		// expectedPublishURL is the upload endpoint, without the /simple suffix.
		expectedPublishURL string
		// expectedResolveURL is the PEP 503 index, which must keep the /simple suffix.
		expectedResolveURL string
	}{
		{
			name:               "Strips /simple suffix for publish only",
			repository:         "poetry-local",
			serverURL:          "https://my-server.jfrog.io/artifactory",
			username:           "user",
			password:           "pass",
			expectedPublishURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
			expectedResolveURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local/simple",
		},
		{
			name:               "Handles different repository name",
			repository:         "poetry-remote",
			serverURL:          "https://my-server.jfrog.io/artifactory",
			username:           "user",
			password:           "pass",
			expectedPublishURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-remote",
			expectedResolveURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-remote/simple",
		},
		{
			name:       "Works with access token",
			repository: "poetry-local",
			serverURL:  "https://my-server.jfrog.io/artifactory",
			// #nosec G101 -- This is a fake test token with no real credentials.
			accessToken:        "fake-test-token-for-unit-testing-only", //nolint:gosec
			expectedPublishURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
			expectedResolveURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local/simple",
		},
		{
			name:               "Handles server URL with trailing slash",
			repository:         "poetry-local",
			serverURL:          "https://my-server.jfrog.io/artifactory/",
			username:           "user",
			password:           "pass",
			expectedPublishURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
			expectedResolveURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local/simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server details
			serverDetails := &config.ServerDetails{}
			serverDetails.ArtifactoryUrl = tt.serverURL
			serverDetails.User = tt.username
			serverDetails.Password = tt.password
			serverDetails.AccessToken = tt.accessToken

			// Get URL with credentials - this returns URL with /simple suffix
			rtUrl, _, password, err := GetPypiRepoUrlWithCredentials(serverDetails, tt.repository, false)
			require.NoError(t, err)
			require.NotEmpty(t, password)

			publishUrl := poetryRepoUrl(rtUrl, "publish")
			assert.Equal(t, tt.expectedPublishURL, publishUrl)
			assert.NotContains(t, publishUrl, "/simple", "publish URL should not contain /simple")
			assert.False(t, strings.HasSuffix(publishUrl, "/"), "publish URL should not have trailing slash")

			// Resolution commands write this URL to [[tool.poetry.source]] in pyproject.toml.
			// Poetry queries it as a PEP 503 index, so /simple must be preserved.
			for _, commandName := range []string{"install", "add", "update", "lock", ""} {
				resolveUrl := poetryRepoUrl(rtUrl, commandName)
				assert.Equal(t, tt.expectedResolveURL, resolveUrl,
					"resolve URL should keep /simple for command %q", commandName)
			}
		})
	}
}
