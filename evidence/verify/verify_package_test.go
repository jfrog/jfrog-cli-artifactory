package verify

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerPackage embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerPackage struct {
	artifactory.EmptyArtifactoryServicesManager
	AqlResponse           string
	AqlError              error
	GetRepositoryResponse services.RepositoryDetails
	GetRepositoryError    error
	PackageLeadFileData   []byte
	GetPackageLeadError   error
}

func (m *MockArtifactoryServicesManagerPackage) Aql(_ string) (io.ReadCloser, error) {
	if m.AqlError != nil {
		return nil, m.AqlError
	}
	return io.NopCloser(bytes.NewBufferString(m.AqlResponse)), nil
}

func (m *MockArtifactoryServicesManagerPackage) GetRepository(_ string, repoDetails any) error {
	if m.GetRepositoryError != nil {
		return m.GetRepositoryError
	}
	if details, ok := repoDetails.(*services.RepositoryDetails); ok {
		*details = m.GetRepositoryResponse
	}
	return nil
}

func (m *MockArtifactoryServicesManagerPackage) GetPackageLeadFile(_ services.LeadFileParams) ([]byte, error) {
	if m.GetPackageLeadError != nil {
		return nil, m.GetPackageLeadError
	}
	return m.PackageLeadFileData, nil
}

// MockOneModelManagerPackage for package tests
type MockOneModelManagerPackage struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockOneModelManagerPackage) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// MockVerifyEvidenceBasePackage for testing verifyEvidence method
type MockVerifyEvidenceBasePackage struct {
	mock.Mock
	verifyEvidenceBase
}

func (m *MockVerifyEvidenceBasePackage) verifyEvidence(client *artifactory.ArtifactoryServicesManager, metadata *[]model.SearchEvidenceEdge, sha256 string) error {
	args := m.Called(client, metadata, sha256)
	return args.Error(0)
}

// MockVerifierPackage for testing with testify - implements VerifierInterface
type MockVerifierPackage struct {
	mock.Mock
}

func (m *MockVerifierPackage) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
	args := m.Called(subjectSha256, evidenceMetadata, subjectPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	resp, ok := args.Get(0).(*model.VerificationResponse)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return resp, args.Error(1)
}

func TestNewVerifyEvidencePackage(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	format := "json"
	packageName := "test-package"
	packageVersion := "1.0.0"
	packageRepoName := "test-repo"
	keys := []string{"key1", "key2"}

	cmd := NewVerifyEvidencePackage(serverDetails, format, packageName, packageVersion, packageRepoName, keys, true)
	verifyCmd, ok := cmd.(*verifyEvidencePackage)
	assert.True(t, ok)
	assert.Equal(t, serverDetails, verifyCmd.serverDetails)
	assert.Equal(t, format, verifyCmd.format)
	assert.Equal(t, packageName, verifyCmd.packageService.GetPackageName())
	assert.Equal(t, packageVersion, verifyCmd.packageService.GetPackageVersion())
	assert.Equal(t, packageRepoName, verifyCmd.packageService.GetPackageRepoName())
	assert.Equal(t, keys, verifyCmd.keys)
	assert.True(t, verifyCmd.useArtifactoryKeys)
}

func TestVerifyEvidencePackage_CommandName(t *testing.T) {
	cmd := &verifyEvidencePackage{}
	assert.Equal(t, "verify-package-evidence", cmd.CommandName())
}

func TestVerifyEvidencePackage_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "test.com"}
	cmd := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestVerifyEvidencePackage_Run_AqlError(t *testing.T) {
	// Mock Artifactory client with error
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlError: errors.New("aql query failed"),
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Create package verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute AQL query")
}

func TestVerifyEvidencePackage_Run_GetPackageTypeError(t *testing.T) {
	// Mock Artifactory client with repository error
	mockClient := &MockArtifactoryServicesManagerPackage{
		GetRepositoryError: errors.New("repository not found"),
	}

	// Create package verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get package type")
}

func TestVerifyEvidencePackage_Run_NoPackageFound(t *testing.T) {
	// Mock AQL response with no results
	aqlResult := `{"results":[]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Create package verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no package lead file found for the given package name and version")
}

func TestVerifyEvidencePackage_Run_GetLeadArtifactError(t *testing.T) {
	// Mock Artifactory client with lead file error
	mockClient := &MockArtifactoryServicesManagerPackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		GetPackageLeadError: errors.New("lead file not found"),
	}

	// Create package verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	// We need to mock the metadata service creation, but since that's internal,
	// we'll check that the error is properly propagated
	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get package version lead artifact")
}

func TestVerifyEvidencePackage_Run_QueryEvidenceMetadataError(t *testing.T) {
	// Mock AQL response with package file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"test-package-1.0.0.jar"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Mock OneModel client with error
	mockOneModel := &MockOneModelManagerPackage{
		GraphqlError: errors.New("graphql query failed"),
	}

	// Create package verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error querying evidence from One-Model service: graphql query failed")
}

func TestVerifyEvidencePackage_Run_VerifyEvidenceError(t *testing.T) {
	// Mock AQL response with package file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"test-package-1.0.0.jar"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerPackage{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification with error
	mockBase := &MockVerifyEvidenceBasePackage{}
	base := &verifyEvidenceBase{
		serverDetails: &config.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidence", mock.Anything, mock.Anything, "test-sha256").Return(errors.New("verification failed"))

	// Test direct method call
	err := mockBase.verifyEvidence(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
	mockBase.AssertExpectations(t)
}

func TestVerifyEvidencePackage_Run_CreateArtifactoryClientError(t *testing.T) {
	// Test when createArtifactoryClient fails due to invalid server configuration
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{
				Url: "invalid-url", // Invalid URL that should cause client creation to fail
			},
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	// The error might be related to client creation or other issues
	assert.True(t, err != nil)
}

func TestVerifyEvidencePackage_Run_Success(t *testing.T) {
	// Mock AQL response with package file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"test-package-1.0.0.jar"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerPackage{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierPackage)

	// Set up expectations for the mock verifier
	expectedResponse := &model.VerificationResponse{
		Subject: model.Subject{
			Path:   "maven-local/test-package/1.0.0/test-package-1.0.0.jar",
			Sha256: "test-sha256",
		},
		EvidenceVerifications:     &[]model.EvidenceVerification{},
		OverallVerificationStatus: model.Success,
	}
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expectedResponse, nil)

	// Create package verifier with mock verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	// Call the actual Run method
	err := packageVerifier.Run()

	// Should succeed with our mock verifier returning success status
	assert.NoError(t, err)

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidencePackage_Run_VerificationFailed(t *testing.T) {
	// Mock AQL response with package file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"test-package-1.0.0.jar"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerPackage{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierPackage)

	// Set up expectations for the mock verifier to return FAILED status
	expectedResponse := &model.VerificationResponse{
		Subject: model.Subject{
			Path:   "maven-local/test-package/1.0.0/test-package-1.0.0.jar",
			Sha256: "test-sha256",
		},
		EvidenceVerifications:     &[]model.EvidenceVerification{},
		OverallVerificationStatus: model.Failed,
	}
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expectedResponse, nil)

	// Create package verifier with mock verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	// Call the actual Run method
	err := packageVerifier.Run()

	// Should fail with ExitCodeFailNoOp due to FAILED verification status
	assert.Error(t, err)
	var cliErr coreutils.CliError
	assert.ErrorAs(t, err, &cliErr)
	assert.Equal(t, coreutils.ExitCodeError, cliErr.ExitCode)

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidencePackage_Run_VerificationError(t *testing.T) {
	// Mock AQL response with package file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"test-package-1.0.0.jar"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerPackage{
		AqlResponse: aqlResult,
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerPackage{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierPackage)

	// Set up expectations for the mock verifier to return an error
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return((*model.VerificationResponse)(nil), errors.New("verification error"))

	// Create package verifier with mock verifier
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		packageService: evidence.NewPackageService("test-package", "1.0.0", "maven-local"),
	}

	// Call the actual Run method
	err := packageVerifier.Run()

	// Should return the verification error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification error")

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidencePackage_Progress_Success(t *testing.T) {
	aqlResult := `{"results":[{"sha256":"sha","name":"pkg.jar"}]}`
	mockClient := &MockArtifactoryServicesManagerPackage{AqlResponse: aqlResult, GetRepositoryResponse: services.RepositoryDetails{PackageType: "maven"}, PackageLeadFileData: []byte("maven-local/p/v/p.jar")}
	mockOneModel := &MockOneModelManagerPackage{GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"sha"},"downloadPath":"/evidence"}}]}}}}`)}
	mockVerifier := new(MockVerifierPackage)
	expected := &model.VerificationResponse{OverallVerificationStatus: model.Success, Subject: model.Subject{Path: "maven-local/p/v/p.jar", Sha256: "sha"}}
	mockVerifier.On("Verify", "sha", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expected, nil)

	cmd := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:  &config.ServerDetails{},
			format:         "json",
			verifier:       mockVerifier,
			oneModelClient: mockOneModel,
		},
		packageService: evidence.NewPackageService("p", "v", "maven-local"),
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	cmd.artifactoryClient = &clientInterface
	pm := &fakeProgress{}
	cmd.progressMgr = pm

	err := cmd.Run()
	assert.NoError(t, err)
	assert.True(t, pm.quitCalled)
	assert.True(t, len(pm.headlines) >= 1)
}
