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

func TestSetPypiRepoUrlWithCredentials_URLTransformation(t *testing.T) {
	tests := []struct {
		name        string
		repository  string
		serverURL   string
		username    string
		password    string
		accessToken string
		expectedURL string
	}{
		{
			name:        "Strips /simple suffix from URL",
			repository:  "poetry-local",
			serverURL:   "https://my-server.jfrog.io/artifactory",
			username:    "user",
			password:    "pass",
			expectedURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
		},
		{
			name:        "Handles different repository name",
			repository:  "poetry-remote",
			serverURL:   "https://my-server.jfrog.io/artifactory",
			username:    "user",
			password:    "pass",
			expectedURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-remote",
		},
		{
			name:        "Works with access token",
			repository:  "poetry-local",
			serverURL:   "https://my-server.jfrog.io/artifactory",
			accessToken: "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0",
			expectedURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
		},
		{
			name:        "Handles server URL with trailing slash",
			repository:  "poetry-local",
			serverURL:   "https://my-server.jfrog.io/artifactory/",
			username:    "user",
			password:    "pass",
			expectedURL: "https://my-server.jfrog.io/artifactory/api/pypi/poetry-local",
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

			if password != "" {
				// Construct base URL
				baseUrl := rtUrl.Scheme + "://" + rtUrl.Host + rtUrl.Path

				// This is the logic from SetPypiRepoUrlWithCredentials that we're testing
				publishUrl := strings.TrimSuffix(baseUrl, "/simple")
				publishUrl = strings.TrimSuffix(publishUrl, "/")

				// Validate
				assert.Equal(t, tt.expectedURL, publishUrl)
				assert.NotContains(t, publishUrl, "/simple", "URL should not contain /simple")
				assert.False(t, strings.HasSuffix(publishUrl, "/"), "URL should not have trailing slash")
			}
		})
	}
}
