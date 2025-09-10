package create

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	artutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	evdservices "github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"
)

func TestNewCreateEvidencePackage(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	predicateFilePath := "/path/to/predicate.json"
	predicateType := "custom-predicate"
	markdownFilePath := "/path/to/markdown.md"
	key := "test-key"
	keyId := "test-key-id"
	packageName := "test-package"
	packageVersion := "1.0.0"
	packageRepoName := "test-repo"

	cmd := NewCreateEvidencePackage(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, packageName, packageVersion, packageRepoName, "", "")
	createCmd, ok := cmd.(*createEvidencePackage)
	assert.True(t, ok)

	// Test createEvidenceBase fields
	assert.Equal(t, serverDetails, createCmd.serverDetails)
	assert.Equal(t, predicateFilePath, createCmd.predicateFilePath)
	assert.Equal(t, predicateType, createCmd.predicateType)
	assert.Equal(t, markdownFilePath, createCmd.markdownFilePath)
	assert.Equal(t, key, createCmd.key)
	assert.Equal(t, keyId, createCmd.keyId)

	// Test packageService fields
	assert.Equal(t, packageName, createCmd.packageService.GetPackageName())
	assert.Equal(t, packageVersion, createCmd.packageService.GetPackageVersion())
	assert.Equal(t, packageRepoName, createCmd.packageService.GetPackageRepoName())
}

func TestCreateEvidencePackage_CommandName(t *testing.T) {
	cmd := &createEvidencePackage{}
	assert.Equal(t, "create-package-evidence", cmd.CommandName())
}

func TestCreateEvidencePackage_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com"}
	cmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestCreateEvidencePackage_RecordSummary(t *testing.T) {
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

	serverDetails := &config.ServerDetails{
		Url:      "http://test.com",
		User:     "testuser",
		Password: "testpass",
	}

	packageName := "test-package"
	packageVersion := "1.0.0"
	repoName := "maven-local"

	evidence := NewCreateEvidencePackage(
		serverDetails,
		"",
		"test-predicate-type",
		"",
		"test-key",
		"test-key-id",
		packageName,
		packageVersion,
		repoName,
		"",
		"",
	)
	c, ok := evidence.(*createEvidencePackage)
	assert.True(t, ok, "should create createEvidencePackage instance")

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "test-slug",
		Verified:      false,
	}
	expectedArtifactPath := "maven-local/test-package/1.0.0/test-package-1.0.0.jar"
	expectedSha256 := "package-sha256"

	c.recordSummary(expectedResponse, expectedArtifactPath, expectedSha256)

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

			assert.Equal(t, expectedArtifactPath, summaryData.Subject)
			assert.Equal(t, expectedSha256, summaryData.SubjectSha256)
			assert.Equal(t, "test-predicate-type", summaryData.PredicateType)
			assert.Equal(t, "test-slug", summaryData.PredicateSlug)
			assert.False(t, summaryData.Verified)
			assert.Equal(t, "test-package 1.0.0", summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypePackage, summaryData.SubjectType)
			assert.Equal(t, repoName, summaryData.RepoKey)
			break
		}
	}
}

func TestCreateEvidencePackage_ProviderId(t *testing.T) {
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

			cmd := NewCreateEvidencePackage(
				serverDetails,
				"",
				"test-predicate-type",
				"",
				"test-key",
				"test-key-id",
				"test-package",
				"1.0.0",
				"test-repo",
				tt.providerId,
				"",
			)

			createCmd, ok := cmd.(*createEvidencePackage)
			assert.True(t, ok)

			// Verify that the integration ID is correctly set in the base struct
			assert.Equal(t, tt.expectedProviderId, createCmd.providerId)
		})
	}
}

type fakeMetadata struct {
	resp []byte
	err  error
}

func (f *fakeMetadata) GraphqlQuery(q []byte) ([]byte, error) { return f.resp, f.err }

type fakePackageService struct{ name, version, repo, pkgType, lead string }

func (f *fakePackageService) GetPackageType(_ artifactory.ArtifactoryServicesManager) (string, error) {
	return f.pkgType, nil
}
func (f *fakePackageService) GetPackageVersionLeadArtifact(_ string, _ metadata.Manager, _ artifactory.ArtifactoryServicesManager) (string, error) {
	return f.lead, nil
}
func (f *fakePackageService) GetPackageName() string     { return f.name }
func (f *fakePackageService) GetPackageVersion() string  { return f.version }
func (f *fakePackageService) GetPackageRepoName() string { return f.repo }

// createMockArtifactoryManagerForPackageTests creates a mock with standard package test behavior
func createMockArtifactoryManagerForPackageTests() *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		FileInfoFunc: func(_ string) (*artutils.FileInfo, error) {
			return NewFileInfoBuilder().
				WithSha256("sha").
				Build(), nil
		},
	}
}

type mockUploaderPkg struct{ body []byte }

func (m *mockUploaderPkg) UploadEvidence(details evdservices.EvidenceDetails) ([]byte, error) {
	resp := model.CreateResponse{PredicateSlug: "slug", Verified: true, PredicateType: "t"}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	m.body = details.DSSEFileRaw
	return b, nil
}

func TestCreateEvidencePackage_Run_WithInjectedDeps(t *testing.T) {
	keyContent, err := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	assert.NoError(t, err)

	// Create temp predicate file
	dir := t.TempDir()
	predPath := filepath.Join(dir, "predicate.json")
	assert.NoError(t, os.WriteFile(predPath, []byte(`{"x":1}`), 0600))

	art := createMockArtifactoryManagerForPackageTests()
	upl := &mockUploaderPkg{}

	c := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{User: "u"},
			predicateFilePath: predPath,
			predicateType:     "https://in-toto.io/Statement/v1",
			key:               string(keyContent),
			artifactoryClient: art,
			uploader:          upl,
		},
		packageService: &fakePackageService{name: "n", version: "1.0.0", repo: "r", pkgType: "maven", lead: "r/n/1.0.0/n-1.0.0.jar"},
		metadataClient: &fakeMetadata{},
	}

	err = c.Run()
	assert.NoError(t, err)
	assert.NotNil(t, upl.body)
}

type fakePackageServiceTypeErr struct{ fakePackageService }

func (f *fakePackageServiceTypeErr) GetPackageType(_ artifactory.ArtifactoryServicesManager) (string, error) {
	return "", assert.AnError
}

type fakePackageServiceLeadErr struct{ fakePackageService }

func (f *fakePackageServiceLeadErr) GetPackageVersionLeadArtifact(_ string, _ metadata.Manager, _ artifactory.ArtifactoryServicesManager) (string, error) {
	return "", assert.AnError
}

// createMockWithFileInfoErrorForPackage creates a mock that returns error for FileInfo
func createMockWithFileInfoErrorForPackage() *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		FileInfoFunc: func(_ string) (*artutils.FileInfo, error) {
			return nil, assert.AnError
		},
	}
}

type failingUploaderPkg struct{ err error }

func (f failingUploaderPkg) UploadEvidence(e evdservices.EvidenceDetails) ([]byte, error) {
	return nil, f.err
}

func TestCreateEvidencePackage_Run_GetPackageTypeError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createMockArtifactoryManagerForPackageTests()
	c := &createEvidencePackage{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art}, packageService: &fakePackageServiceTypeErr{fakePackageService{}}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // GetPackageType error
}

func TestCreateEvidencePackage_Run_GetLeadArtifactError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createMockArtifactoryManagerForPackageTests()
	c := &createEvidencePackage{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art}, packageService: &fakePackageServiceLeadErr{fakePackageService{pkgType: "maven"}}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // GetPackageVersionLeadArtifact error
}

func TestCreateEvidencePackage_Run_FileInfoError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createMockWithFileInfoErrorForPackage()
	c := &createEvidencePackage{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art}, packageService: &fakePackageService{pkgType: "maven", lead: "r/n/1.0.0/n-1.0.0.jar"}, metadataClient: &fakeMetadata{}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // FileInfo error
}

func TestCreateEvidencePackage_Run_EnvelopeError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	pub, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/public_key.pem"))
	art := createMockArtifactoryManagerForPackageTests()
	c := &createEvidencePackage{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(pub), artifactoryClient: art}, packageService: &fakePackageService{pkgType: "maven", lead: "r/n/1.0.0/n-1.0.0.jar"}, metadataClient: &fakeMetadata{}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load private key") // Public key cannot be used for signing
}

func TestCreateEvidencePackage_Run_UploadError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	_ = os.WriteFile(pred, []byte(`{"a":1}`), 0600)
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createMockArtifactoryManagerForPackageTests()
	upl := failingUploaderPkg{err: assert.AnError}
	c := &createEvidencePackage{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art, uploader: upl}, packageService: &fakePackageService{pkgType: "maven", lead: "r/n/1.0.0/n-1.0.0.jar"}, metadataClient: &fakeMetadata{}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assert.AnError general error for testing") // Upload error
}
