package create

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evdservices "github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/stretchr/testify/assert"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
)

func createMockArtifactoryManagerForBuildTests() *SimpleMockServicesManager {
	return PrepareMockArtifactoryManagerForBuildTests()
}

func TestBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		project          string
		buildName        string
		buildNumber      string
		expectedPath     string
		expectedChecksum string
		expectError      bool
	}{
		{
			name:             "Valid buildName with project",
			project:          "myProject",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "myProject-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
		{
			name:             "Valid buildName default project",
			project:          "default",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "artifactory-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := NewCreateEvidenceBuild(nil, "", "", "", "", "", tt.project, tt.buildName, tt.buildNumber, "", "").(*createEvidenceBuild)
			assert.True(t, ok, "should create createEvidenceBuild instance")
			aa := createMockArtifactoryManagerForBuildTests()
			timestamp, err := getBuildLatestTimestamp(tt.buildName, tt.buildNumber, tt.project, aa)
			assert.NoError(t, err)
			path, sha256, err := c.buildBuildInfoSubjectPath(aa, timestamp)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, path)
				assert.Empty(t, sha256)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedChecksum, sha256)
			}
		})
	}
}

type mockUploader struct{ body []byte }

func (m *mockUploader) UploadEvidence(details evdservices.EvidenceDetails) ([]byte, error) {
	resp := model.CreateResponse{PredicateSlug: "slug", Verified: true, PredicateType: "t"}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	m.body = details.DSSEFileRaw
	return b, nil
}

func TestCreateEvidenceBuild_Run_WithInjectedDeps(t *testing.T) {
	keyContent, err := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	assert.NoError(t, err)

	// Create temp predicate file
	dir := t.TempDir()
	predPath := filepath.Join(dir, "predicate.json")
	assert.NoError(t, os.WriteFile(predPath, []byte(`{"x":1}`), 0600))

	art := createMockArtifactoryManagerForBuildTests()
	upl := &mockUploader{}

	c := &createEvidenceBuild{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{User: "u"},
			predicateFilePath: predPath,
			predicateType:     "https://in-toto.io/Statement/v1",
			key:               string(keyContent),
			artifactoryClient: art,
			uploader:          upl,
		},
		project:     "myProject",
		buildName:   "buildName",
		buildNumber: "1",
	}

	err = c.Run()
	assert.NoError(t, err)
	assert.NotNil(t, upl.body)
}

// Additional error-path coverage for create_build

// Use exported functions from test_mocks.go
func createMockWithBuildError(buildErr error, ok bool, started string) *SimpleMockServicesManager {
	return PrepareMockWithBuildError(buildErr, ok, started)
}

func createMockWithFileInfoError() *SimpleMockServicesManager {
	return PrepareMockWithFileInfoError()
}

type failingUploaderBuild struct{ err error }

func (f *failingUploaderBuild) UploadEvidence(e evdservices.EvidenceDetails) ([]byte, error) {
	return nil, f.err
}

func TestCreateEvidenceBuild_Run_ErrorOnGetBuildInfo_ErrorReturned(t *testing.T) {
	artMgr := createMockWithBuildError(assert.AnError, false, "")
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{artifactoryClient: artMgr, predicateFilePath: pred, predicateType: "t", key: string(key)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing")
}

func TestCreateEvidenceBuild_Run_ErrorOnGetBuildInfo_NotFound(t *testing.T) {
	artMgr := createMockWithBuildError(nil, false, "2024-01-17T15:04:05.000-0700")
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{artifactoryClient: artMgr, predicateFilePath: pred, predicateType: "t", key: string(key)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find buildName")
	assert.Contains(t, err.Error(), "name:b, number:1, project: p")
}

func TestCreateEvidenceBuild_Run_ErrorOnGetBuildInfo_ParseTimestamp(t *testing.T) {
	artMgr := createMockWithBuildError(nil, true, "bad-time")
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{artifactoryClient: artMgr, predicateFilePath: pred, predicateType: "t", key: string(key)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing time") // ParseIsoTimestamp returns time parsing error
}

func TestCreateEvidenceBuild_Run_ErrorOnFileInfo(t *testing.T) {
	artMgr := createMockWithFileInfoError()
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{artifactoryClient: artMgr, predicateFilePath: pred, predicateType: "t", key: string(key)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // FileInfo error from mock
}

func TestCreateEvidenceBuild_Run_EnvelopeError(t *testing.T) {
	pubKey, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/public_key.pem"))
	artMgr := createMockArtifactoryManagerForBuildTests()
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, artifactoryClient: artMgr, predicateFilePath: pred, predicateType: "t", key: string(pubKey)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load private key") // Public key cannot be used for signing
}

func TestCreateEvidenceBuild_Run_UploadError(t *testing.T) {
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	artMgr := createMockArtifactoryManagerForBuildTests()
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	c := &createEvidenceBuild{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, artifactoryClient: artMgr, uploader: &failingUploaderBuild{err: assert.AnError}, predicateFilePath: pred, predicateType: "t", key: string(key)}, project: "p", buildName: "b", buildNumber: "1"}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // Upload error from failingUploaderBuild
}

func TestCreateEvidenceBuild_RecordSummary(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, fileutils.RemoveTempDir(tempDir))
	}()

	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()

	predicateFile := filepath.Join(tempDir, "predicate.json")
	predicateContent := `{"test": "predicate"}`
	assert.NoError(t, os.WriteFile(predicateFile, []byte(predicateContent), 0644))

	serverDetails := &config.ServerDetails{
		Url:      "http://test.com",
		User:     "testuser",
		Password: "testpass",
	}

	evidence := NewCreateEvidenceBuild(
		serverDetails,
		predicateFile,
		"test-predicate-type",
		"",
		"test-key",
		"test-key-id",
		"myProject",
		"testBuild",
		"123",
		"",
		"",
	)
	c, ok := evidence.(*createEvidenceBuild)
	assert.True(t, ok, "should create createEvidenceBuild instance")

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "test-slug",
		Verified:      true,
	}
	expectedTimestamp := "1705529045000"
	expectedSubject := "myProject-build-info/testBuild/123-1705529045000.json"
	expectedSha256 := "test-sha256"

	c.recordSummary(expectedSubject, expectedSha256, expectedResponse, expectedTimestamp)

	summaryFiles, err := fileutils.ListFiles(tempDir, true)
	assert.NoError(t, err)
	assert.True(t, len(summaryFiles) > 0, "Summary file should be created")

	for _, file := range summaryFiles {
		if strings.HasSuffix(file, "-data") {
			content, err := os.ReadFile(file)
			assert.NoError(t, err)

			var summaryData commandsummary.EvidenceSummaryData
			err = json.Unmarshal(content, &summaryData)
			assert.NoError(t, err)

			assert.Equal(t, expectedSubject, summaryData.Subject)
			assert.Equal(t, expectedSha256, summaryData.SubjectSha256)
			assert.Equal(t, "test-predicate-type", summaryData.PredicateType)
			assert.Equal(t, "test-slug", summaryData.PredicateSlug)
			assert.True(t, summaryData.Verified)
			assert.Equal(t, "testBuild 123", summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypeBuild, summaryData.SubjectType)
			assert.Equal(t, "testBuild", summaryData.BuildName)
			assert.Equal(t, "123", summaryData.BuildNumber)
			assert.Equal(t, expectedTimestamp, summaryData.BuildTimestamp)
			assert.Equal(t, "myProject-build-info", summaryData.RepoKey)
			break
		}
	}
}

func TestCreateEvidenceBuild_ProviderId(t *testing.T) {
	tests := []struct {
		name               string
		providerId         string
		expectedProviderId string
	}{
		{
			name:               "With custom integration ID",
			providerId:         "custom-integration",
			expectedProviderId: "custom-integration",
		},
		{
			name:               "With empty integration ID",
			providerId:         "",
			expectedProviderId: "",
		},
		{
			name:               "With sonar integration ID",
			providerId:         "sonar",
			expectedProviderId: "sonar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverDetails := &config.ServerDetails{Url: "http://test.com"}

			cmd := NewCreateEvidenceBuild(
				serverDetails,
				"",
				"test-predicate-type",
				"",
				"test-key",
				"test-key-id",
				"test-project",
				"test-build",
				"1",
				tt.providerId,
				"",
			)

			createCmd, ok := cmd.(*createEvidenceBuild)
			assert.True(t, ok)

			// Verify that the integration ID is correctly set in the base struct
			assert.Equal(t, tt.expectedProviderId, createCmd.providerId)
		})
	}
}
