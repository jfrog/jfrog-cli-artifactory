package create

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	artifactoryUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/froggit-go/vcsclient"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	artservices "github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	evdservices "github.com/jfrog/jfrog-client-go/evidence/services"
)

// MockBuildAndVcsDetails is a mock implementation of the BuildAndVcsDetails interface
type MockBuildAndVcsDetails struct {
	mock.Mock
}

func (m *MockBuildAndVcsDetails) GetBuildDetails() *build.BuildConfiguration {
	args := m.Called()
	if buildConfig := args.Get(0); buildConfig != nil {
		if bc, ok := buildConfig.(*build.BuildConfiguration); ok {
			return bc
		}
	}
	return nil
}

func (m *MockBuildAndVcsDetails) GetVcsInfo() (string, string) {
	args := m.Called()
	return args.String(0), args.String(1)
}

// Test cases
func TestGetFlagType(t *testing.T) {
	tests := []struct {
		name     string
		typeFlag string
		expected FlagType
	}{
		{"Empty flag", "", FlagTypeOther},
		{"Committer reviewer flag", "gh-commiter", FlagTypeCommitterReviewer},
		{"Other flag", "some-other-flag", FlagTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFlagType(tt.typeFlag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsRunningUnderGitHubAction removed - testing private function

func TestNewCreateGithub(t *testing.T) {
	serverDetails := &config.ServerDetails{User: "test-user"}
	cmd := NewCreateGithub(serverDetails, "predicate.json", "test-type", "markdown.md", "key", "keyId", "project", "build", "1", "gh-commiter")

	ghEvidence, ok := cmd.(*createGitHubEvidence)
	assert.True(t, ok)
	assert.Equal(t, "project", ghEvidence.project)
	assert.Equal(t, "build", ghEvidence.buildName)
	assert.Equal(t, "1", ghEvidence.buildNumber)
	assert.Equal(t, FlagTypeCommitterReviewer, ghEvidence.flagType)
}

func TestCommitterReviewerEvidence_FlagTypeMismatch(t *testing.T) {
	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{flagType: FlagTypeOther},
	}
	_, err := c.committerReviewerEvidence()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flag type must be gh-commiter")
}

func TestBuildAndVcsDetailsMock(t *testing.T) {
	mockBuildVcs := new(MockBuildAndVcsDetails)

	// Set up expectations
	buildConfig := &build.BuildConfiguration{}
	mockBuildVcs.On("GetBuildDetails").Return(buildConfig)
	mockBuildVcs.On("GetVcsInfo").Return("vcs-url", "vcs-revision")

	// Test the mock
	result := mockBuildVcs.GetBuildDetails()
	assert.Equal(t, buildConfig, result)

	vcsUrl, vcsRevision := mockBuildVcs.GetVcsInfo()
	assert.Equal(t, "vcs-url", vcsUrl)
	assert.Equal(t, "vcs-revision", vcsRevision)

	// Verify expectations were met
	mockBuildVcs.AssertExpectations(t)
}

func TestGitHubRepositoryDetails(t *testing.T) {
	_ = os.Setenv("GITHUB_REPOSITORY", "owner/repo")
	defer func() { _ = os.Unsetenv("GITHUB_REPOSITORY") }()

	owner, repo, err := gitHubRepositoryDetails()
	assert.NoError(t, err)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestGitHubRepositoryDetails_MissingEnvVar(t *testing.T) {
	_ = os.Unsetenv("GITHUB_REPOSITORY")

	_, _, err := gitHubRepositoryDetails()
	assert.Error(t, err)
	// Just check that we get an error, not the specific type
}

func TestGitHubRepositoryDetails_InvalidFormat(t *testing.T) {
	_ = os.Setenv("GITHUB_REPOSITORY", "invalid-format")
	defer func() { _ = os.Unsetenv("GITHUB_REPOSITORY") }()

	_, _, err := gitHubRepositoryDetails()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GITHUB_REPOSITORY format")
}

// Helper types for testing
type fakeArtMgrGH struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (f *fakeArtMgrGH) FileInfo(_ string) (*utils.FileInfo, error) {
	return &utils.FileInfo{Checksums: struct {
		Sha1   string `json:"sha1,omitempty"`
		Sha256 string `json:"sha256,omitempty"`
		Md5    string `json:"md5,omitempty"`
	}{Sha256: "sha256-build-json"}}, nil
}

func (f *fakeArtMgrGH) GetBuildInfo(artservices.BuildInfoParams) (*buildinfo.PublishedBuildInfo, bool, error) {
	return &buildinfo.PublishedBuildInfo{BuildInfo: buildinfo.BuildInfo{Started: "2024-01-17T15:04:05.000-0700"}}, true, nil
}

type captureUploaderGH struct{ body []byte }

func (c *captureUploaderGH) UploadEvidence(d evdservices.EvidenceDetails) ([]byte, error) {
	resp := model.CreateResponse{PredicateSlug: "slug", Verified: true}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	c.body = d.DSSEFileRaw
	return b, nil
}

// Simple mock for pullRequestClient interface
type mockPullRequestClient struct {
	prInfo  []vcsclient.PullRequestInfo
	prErr   error
	reviews []vcsclient.PullRequestReviewDetails
	revErr  error
}

func (m *mockPullRequestClient) ListPullRequestsAssociatedWithCommit(ctx context.Context, owner, repository, commit string) ([]vcsclient.PullRequestInfo, error) {
	if m.prErr != nil {
		return nil, m.prErr
	}
	return m.prInfo, nil
}

func (m *mockPullRequestClient) ListPullRequestReviews(ctx context.Context, owner, repository string, prID int) ([]vcsclient.PullRequestReviewDetails, error) {
	if m.revErr != nil {
		return nil, m.revErr
	}
	return m.reviews, nil
}

func TestCreateGithub_Run_Success_WithInjectedDeps(t *testing.T) {
	// Save original functions
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origGetLastBuildLink := getLastBuildLink
	origNewVcsClient := newVcsClient
	defer func() {
		// Restore original functions
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		getLastBuildLink = origGetLastBuildLink
		newVcsClient = origNewVcsClient
	}()

	// Setup test environment
	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	tmp, _ := os.MkdirTemp("", "sum")
	_ = os.Setenv(coreutils.SummaryOutputDirPathEnv, tmp)
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv("GITHUB_REPOSITORY")
		_ = os.Unsetenv("JF_GIT_TOKEN")
		_ = os.Unsetenv(coreutils.SummaryOutputDirPathEnv)
		_ = os.RemoveAll(tmp)
	}()

	d := t.TempDir()
	pred := filepath.Join(d, "p.json")
	_ = os.WriteFile(pred, []byte(`{"k":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))

	// Override functions for test
	logEntry := `'{"commit":"c1","subject":"s"}'\n'{"commit":"c2","subject":"s2"}'`
	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return logEntry, nil
	}
	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "http://link", nil
	}
	newVcsClient = func(token string) (pullRequestClient, error) {
		return &mockPullRequestClient{
			prInfo:  []vcsclient.PullRequestInfo{{ID: 7}},
			reviews: []vcsclient.PullRequestReviewDetails{{Reviewer: "r", State: "APPROVED"}},
		}, nil
	}

	upl := &captureUploaderGH{}
	art := &fakeArtMgrGH{}

	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{User: "u"},
			predicateFilePath: pred,
			predicateType:     "t",
			key:               string(key),
			artifactoryClient: art,
			uploader:          upl,
			flagType:          FlagTypeCommitterReviewer,
		},
		project:     "proj",
		buildName:   "b",
		buildNumber: "1",
	}

	err := c.Run()
	assert.NoError(t, err)
	assert.NotNil(t, upl.body)
}

func TestCreateGithub_Run_NotGhActions_Error(t *testing.T) {
	_ = os.Unsetenv("GITHUB_ACTIONS")
	c := &createGitHubEvidence{}
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateGithub_buildBuildInfoSubjectPath_Errors(t *testing.T) {
	// Using exported mocks from test_mocks.go
	badArt := PrepareMockWithBuildError(assert.AnError, false, "")
	c := &createGitHubEvidence{}
	_, _, err := c.buildBuildInfoSubjectPath(badArt)
	assert.Error(t, err)

	nf := PrepareMockWithBuildError(nil, false, "2024-01-17T15:04:05.000-0700")
	_, _, err = c.buildBuildInfoSubjectPath(nf)
	assert.Error(t, err)

	fiErr := PrepareMockWithFileInfoError()
	_, _, err = c.buildBuildInfoSubjectPath(fiErr)
	assert.Error(t, err)
}

func TestCreateGithub_getGitCommitEntries_JSONError(t *testing.T) {
	// Save and override
	orig := getPlainGitLogFromPreviousBuild
	defer func() { getPlainGitLogFromPreviousBuild = orig }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":}'`, nil
	}

	c := &createGitHubEvidence{}
	_, err := c.getGitCommitEntries(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.Error(t, err)
}

func TestCreateGithub_getGitCommitInfo_VcsErrorsHandled(t *testing.T) {
	// Save and override
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origNewVcsClient := newVcsClient
	defer func() {
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		newVcsClient = origNewVcsClient
	}()

	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	defer func() { _ = os.Unsetenv("GITHUB_REPOSITORY"); _ = os.Unsetenv("JF_GIT_TOKEN") }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":"c1"}'`, nil
	}
	newVcsClient = func(token string) (pullRequestClient, error) {
		return &mockPullRequestClient{
			prErr:  assert.AnError,
			revErr: assert.AnError,
		}, nil
	}

	c := &createGitHubEvidence{}
	b, err := c.getGitCommitInfo(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.NoError(t, err)
	re := regexp.MustCompile("c1")
	assert.True(t, re.Match(b))
}

func TestCreateGithub_Run_EnvelopeError(t *testing.T) {
	// Save and override
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origGetLastBuildLink := getLastBuildLink
	defer func() {
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		getLastBuildLink = origGetLastBuildLink
	}()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv("GITHUB_REPOSITORY")
		_ = os.Unsetenv("JF_GIT_TOKEN")
	}()

	d := t.TempDir()
	pred := filepath.Join(d, "p.json")
	_ = os.WriteFile(pred, []byte(`{"k":1}`), 0600)
	pub, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/public_key.pem"))

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":"c1"}'`, nil
	}
	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "l", nil
	}

	art := &fakeArtMgrGH{}

	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{User: "u"}, predicateFilePath: pred, predicateType: "t", key: string(pub), artifactoryClient: art},
		project:            "p",
		buildName:          "b",
		buildNumber:        "1",
	}
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateGithub_Run_UploadError(t *testing.T) {
	// Save and override
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origGetLastBuildLink := getLastBuildLink
	defer func() {
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		getLastBuildLink = origGetLastBuildLink
	}()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv("GITHUB_REPOSITORY")
		_ = os.Unsetenv("JF_GIT_TOKEN")
	}()

	d := t.TempDir()
	pred := filepath.Join(d, "p.json")
	_ = os.WriteFile(pred, []byte(`{"k":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":"c1"}'`, nil
	}
	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "l", nil
	}

	art := &fakeArtMgrGH{}
	upl := &failingUploaderBuild{err: assert.AnError}

	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{User: "u"}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art, uploader: upl},
		project:            "p",
		buildName:          "b",
		buildNumber:        "1",
	}
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateGithub_recordEvidenceSummaryData_Writes(t *testing.T) {
	// Save and override
	orig := getLastBuildLink
	defer func() { getLastBuildLink = orig }()

	tempDir, _ := os.MkdirTemp("", "sum")
	defer func() { _ = os.RemoveAll(tempDir) }()
	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir)
	defer func() { _ = os.Unsetenv("GITHUB_ACTIONS"); _ = os.Unsetenv(coreutils.SummaryOutputDirPathEnv) }()

	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "http://link", nil
	}

	c := &createGitHubEvidence{
		buildName: "b",
	}
	payload := []byte(`[ ]`)
	err := c.recordEvidenceSummaryData(payload, "p", "s")
	assert.NoError(t, err)
	files, _ := os.ReadDir(tempDir)
	assert.True(t, len(files) > 0)
}

func TestCreateGithub_Run_ErrorOnCommitterEvidence(t *testing.T) {
	// Save and override
	orig := getPlainGitLogFromPreviousBuild
	defer func() { getPlainGitLogFromPreviousBuild = orig }()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	defer func() { _ = os.Unsetenv("GITHUB_ACTIONS"); _ = os.Unsetenv("GITHUB_REPOSITORY") }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'not-json'`, nil
	}

	// No need for predicate/key since it should fail before those paths
	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{flagType: FlagTypeCommitterReviewer},
		project:            "p",
		buildName:          "b",
		buildNumber:        "1",
	}
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateGithub_Run_RecordSummaryError(t *testing.T) {
	// Save and override
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origGetLastBuildLink := getLastBuildLink
	origNewVcsClient := newVcsClient
	defer func() {
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		getLastBuildLink = origGetLastBuildLink
		newVcsClient = origNewVcsClient
	}()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv("GITHUB_REPOSITORY")
		_ = os.Unsetenv("JF_GIT_TOKEN")
	}()

	d := t.TempDir()
	pred := filepath.Join(d, "p.json")
	_ = os.WriteFile(pred, []byte(`{"k":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":"c1"}'`, nil
	}
	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "http://link", nil
	}
	newVcsClient = func(token string) (pullRequestClient, error) {
		return &mockPullRequestClient{}, nil
	}

	upl := &captureUploaderGH{}
	art := &fakeArtMgrGH{}

	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{User: "u"},
			predicateFilePath: pred,
			predicateType:     "t",
			key:               string(key),
			artifactoryClient: art,
			uploader:          upl,
			flagType:          FlagTypeCommitterReviewer,
		},
		project:     "proj",
		buildName:   "b",
		buildNumber: "1",
	}

	// Intentionally DO NOT set summary output dir so recordEvidenceSummaryData fails
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateGithub_getGitCommitInfo_MissingRepoEnv(t *testing.T) {
	// Save and override
	orig := getPlainGitLogFromPreviousBuild
	defer func() { getPlainGitLogFromPreviousBuild = orig }()

	_ = os.Unsetenv("GITHUB_REPOSITORY")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	defer func() { _ = os.Unsetenv("JF_GIT_TOKEN") }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{}'`, nil
	}

	c := &createGitHubEvidence{}
	_, err := c.getGitCommitInfo(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.Error(t, err)
}

func TestCreateGithub_getGitCommitInfo_MissingToken(t *testing.T) {
	// Save and override
	orig := getPlainGitLogFromPreviousBuild
	defer func() { getPlainGitLogFromPreviousBuild = orig }()

	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Unsetenv("JF_GIT_TOKEN")
	defer func() { _ = os.Unsetenv("GITHUB_REPOSITORY") }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{}'`, nil
	}

	c := &createGitHubEvidence{}
	_, err := c.getGitCommitInfo(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.Error(t, err)
}

func TestCreateGithub_getGitCommitEntries_FetchError(t *testing.T) {
	// Save and override
	orig := getPlainGitLogFromPreviousBuild
	defer func() { getPlainGitLogFromPreviousBuild = orig }()

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return "", assert.AnError
	}

	c := &createGitHubEvidence{}
	_, err := c.getGitCommitEntries(nil, nil, artifactoryUtils.GitLogDetails{})
	assert.Error(t, err)
}

func TestCreateGithub_getLastBuildLink_Error(t *testing.T) {
	// Save and override
	orig := getLastBuildLink
	defer func() { getLastBuildLink = orig }()

	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "", assert.AnError
	}

	c := &createGitHubEvidence{}
	_, err := c.getLastBuildLink()
	assert.Error(t, err)
}

func TestCreateGithub_recordEvidenceSummaryData_InvalidEvidence(t *testing.T) {
	_ = os.Setenv("GITHUB_ACTIONS", "true")
	defer func() { _ = os.Unsetenv("GITHUB_ACTIONS") }()
	c := &createGitHubEvidence{}
	err := c.recordEvidenceSummaryData([]byte("not-json"), "p", "s")
	assert.Error(t, err)
}

func TestCreateGithub_recordEvidenceSummaryData_LinkError(t *testing.T) {
	// Save and override
	orig := getLastBuildLink
	defer func() { getLastBuildLink = orig }()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	tmp, _ := os.MkdirTemp("", "sum")
	_ = os.Setenv(coreutils.SummaryOutputDirPathEnv, tmp)
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv(coreutils.SummaryOutputDirPathEnv)
		_ = os.RemoveAll(tmp)
	}()

	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "", assert.AnError
	}

	c := &createGitHubEvidence{}
	err := c.recordEvidenceSummaryData([]byte(`[]`), "p", "s")
	assert.Error(t, err)
}

// Mock removed - using PrepareMockWithBuildError from test_mocks.go instead

func TestCreateGithub_Run_BuildSubjectError(t *testing.T) {
	// Save and override
	origGetPlainGitLog := getPlainGitLogFromPreviousBuild
	origGetLastBuildLink := getLastBuildLink
	origNewVcsClient := newVcsClient
	defer func() {
		getPlainGitLogFromPreviousBuild = origGetPlainGitLog
		getLastBuildLink = origGetLastBuildLink
		newVcsClient = origNewVcsClient
	}()

	_ = os.Setenv("GITHUB_ACTIONS", "true")
	_ = os.Setenv("GITHUB_REPOSITORY", "acme/repo")
	_ = os.Setenv("JF_GIT_TOKEN", "t")
	tmp, _ := os.MkdirTemp("", "sum")
	_ = os.Setenv(coreutils.SummaryOutputDirPathEnv, tmp)
	defer func() {
		_ = os.Unsetenv("GITHUB_ACTIONS")
		_ = os.Unsetenv("GITHUB_REPOSITORY")
		_ = os.Unsetenv("JF_GIT_TOKEN")
		_ = os.Unsetenv(coreutils.SummaryOutputDirPathEnv)
		_ = os.RemoveAll(tmp)
	}()

	d := t.TempDir()
	pred := filepath.Join(d, "p.json")
	_ = os.WriteFile(pred, []byte(`{"k":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))

	getPlainGitLogFromPreviousBuild = func(*config.ServerDetails, *build.BuildConfiguration, artifactoryUtils.GitLogDetails) (string, error) {
		return `'{"commit":"c1"}'`, nil
	}
	getLastBuildLink = func(*config.ServerDetails, *build.BuildConfiguration) (string, error) {
		return "l", nil
	}
	newVcsClient = func(token string) (pullRequestClient, error) {
		return &mockPullRequestClient{}, nil
	}

	art := PrepareMockWithBuildError(assert.AnError, false, "")

	c := &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{User: "u"},
			predicateFilePath: pred,
			predicateType:     "t",
			key:               string(key),
			artifactoryClient: art,
			flagType:          FlagTypeCommitterReviewer,
		},
		project:     "p",
		buildName:   "b",
		buildNumber: "1",
	}
	err := c.Run()
	assert.Error(t, err)
}
