package create

import (
	gofrogcmd "github.com/jfrog/gofrog/io"
	artifactoryUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"os"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBuildAndVcsDetails is a mock implementation of the BuildAndVcsDetails interface
type MockBuildAndVcsDetails struct {
	mock.Mock
}

func (m *MockBuildAndVcsDetails) ParseGitLogFromLastVcsRevision(gitDetails artifactoryUtils.GitLogDetails, logRegExp *gofrogcmd.CmdOutputPattern, lastVcsRevision string) error {
	args := m.Called(gitDetails, logRegExp, lastVcsRevision)
	return args.Error(0)
}

func (m *MockBuildAndVcsDetails) GetPlainGitLogFromPreviousBuild(serverDetails *config.ServerDetails, buildConfiguration *build.BuildConfiguration, gitDetails artifactoryUtils.GitLogDetails) (string, error) {
	args := m.Called(serverDetails, buildConfiguration, gitDetails)
	return args.String(0), args.Error(1)
}

func (m *MockBuildAndVcsDetails) GetLastBuildLink(serverDetails *config.ServerDetails, buildConfiguration *build.BuildConfiguration) (string, error) {
	args := m.Called(serverDetails, buildConfiguration)
	return args.String(0), args.Error(1)
}

func TestNewCreateGithub(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	command := NewCreateGithub(serverDetails, "path/to/predicate.json", "predicateType", "path/to/markdown.md", "key", "keyId", "myProject", "myBuild", "123", "gh-commiter")

	assert.NotNil(t, command)

	cgEvidence, ok := command.(*createGitHubEvidence)
	assert.True(t, ok, "Expected command to be of type *createGitHubEvidence")

	assert.Equal(t, "myProject", cgEvidence.project)
	assert.Equal(t, "myBuild", cgEvidence.buildName)
	assert.Equal(t, "123", cgEvidence.buildNumber)
	assert.Equal(t, FlagTypeCommitterReviewer, cgEvidence.flagType)
}

func TestIsRunningUnderGitHubAction(t *testing.T) {
	_ = os.Setenv("GITHUB_ACTIONS", "true")
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
	}()
	assert.True(t, isRunningUnderGitHubAction())

	_ = os.Setenv("GITHUB_ACTIONS", "false")
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
	}()
	assert.False(t, isRunningUnderGitHubAction())
}

func TestGetFlagType(t *testing.T) {
	assert.Equal(t, FlagTypeCommitterReviewer, getFlagType("gh-commiter"))
	assert.Equal(t, FlagTypeOther, getFlagType("random"))
}

func TestCommitterReviewerEvidence_FlagTypeMismatch(t *testing.T) {
	command := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{flagType: FlagTypeOther},
	}

	_, err := command.committerReviewerEvidence()
	assert.Error(t, err)
	assert.Equal(t, "flag type must be gh-commiter", err.Error())
}

func TestBuildAndVcsDetailsMock(t *testing.T) {
	mockBuildVcs := new(MockBuildAndVcsDetails)

	// Define expected return values
	mockBuildVcs.On("GetLastBuildLink", mock.Anything, mock.Anything).Return("http://mocked-url.com", nil)
	mockBuildVcs.On("GetPlainGitLogFromPreviousBuild", mock.Anything, mock.Anything, mock.Anything).Return("mocked git log", nil)

	// Call the method under test
	url, err := mockBuildVcs.GetLastBuildLink(nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "http://mocked-url.com", url)

	gitLog, err := mockBuildVcs.GetPlainGitLogFromPreviousBuild(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.NoError(t, err)
	assert.Equal(t, "mocked git log", gitLog)

	// Assert that the expected calls were made
	mockBuildVcs.AssertExpectations(t)
}

// Test gitHubRepositoryDetails
func TestGitHubRepositoryDetails(t *testing.T) {
	_ = os.Setenv("GITHUB_REPOSITORY", "jfrog/myrepo")
	defer func() {
		_ = os.Unsetenv("GITHUB_REPOSITORY") // Remove the environment variable
	}()
	owner, repo, err := gitHubRepositoryDetails()

	assert.NoError(t, err)
	assert.Equal(t, "jfrog", owner)
	assert.Equal(t, "myrepo", repo)
}

// Test gitHubRepositoryDetails when env var is missing
func TestGitHubRepositoryDetails_MissingEnvVar(t *testing.T) {
	_ = os.Setenv("GITHUB_REPOSITORY", "")
	defer func() {
		_ = os.Unsetenv("GITHUB_REPOSITORY") // Remove the environment variable
	}()

	_, _, err := gitHubRepositoryDetails()
	assert.Error(t, err)
	assert.Equal(t, "GITHUB_REPOSITORY environment variable is not set", err.Error())
}

// Test gitHubRepositoryDetails with invalid format
func TestGitHubRepositoryDetails_InvalidFormat(t *testing.T) {
	_ = os.Setenv("GITHUB_REPOSITORY", "jfrog")
	defer func() {
		_ = os.Unsetenv("GITHUB_REPOSITORY") // Remove the environment variable
	}()

	_, _, err := gitHubRepositoryDetails()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GITHUB_REPOSITORY format")
}
