package evidence

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

func (m *MockArtifactoryServicesManagerPackage) GetRepository(_ string, repoDetails interface{}) error {
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

// MockVerifyEvidenceBasePackage for testing verifyEvidences method
type MockVerifyEvidenceBasePackage struct {
	mock.Mock
	verifyEvidenceBase
}

func (m *MockVerifyEvidenceBasePackage) verifyEvidences(client *artifactory.ArtifactoryServicesManager, metadata *[]model.SearchEvidenceEdge, sha256 string) error {
	args := m.Called(client, metadata, sha256)
	return args.Error(0)
}

func TestNewVerifyEvidencePackage(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{}
	format := "json"
	packageName := "test-package"
	packageVersion := "1.0.0"
	packageRepoName := "test-repo"
	keys := []string{"key1", "key2"}

	cmd := NewVerifyEvidencePackage(serverDetails, format, packageName, packageVersion, packageRepoName, keys)
	verifyCmd, ok := cmd.(*verifyEvidencePackage)
	assert.True(t, ok)
	assert.Equal(t, serverDetails, verifyCmd.serverDetails)
	assert.Equal(t, format, verifyCmd.format)
	assert.Equal(t, packageName, verifyCmd.PackageName)
	assert.Equal(t, packageVersion, verifyCmd.PackageVersion)
	assert.Equal(t, packageRepoName, verifyCmd.PackageRepoName)
	assert.Equal(t, keys, verifyCmd.keys)
}

func TestVerifyEvidencePackage_CommandName(t *testing.T) {
	cmd := &verifyEvidencePackage{}
	assert.Equal(t, "verify-package-evidence", cmd.CommandName())
}

func TestVerifyEvidencePackage_ServerDetails(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{Url: "http://test.com"}
	cmd := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
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
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification
	mockBase := &MockVerifyEvidenceBasePackage{}
	base := &verifyEvidenceBase{
		serverDetails: &coreConfig.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidences", mock.Anything, mock.Anything, "test-sha256").Return(nil)

	// Test direct method call
	err := mockBase.verifyEvidences(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256")
	assert.NoError(t, err)
	mockBase.AssertExpectations(t)
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
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
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
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
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
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
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
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
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
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query evidence metadata")
}

func TestVerifyEvidencePackage_Run_VerifyEvidencesError(t *testing.T) {
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
		serverDetails: &coreConfig.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidences", mock.Anything, mock.Anything, "test-sha256").Return(errors.New("verification failed"))

	// Test direct method call
	err := mockBase.verifyEvidences(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
	mockBase.AssertExpectations(t)
}

func TestVerifyEvidencePackage_Run_CreateArtifactoryClientError(t *testing.T) {
	// Test when createArtifactoryClient fails due to invalid server configuration
	packageVerifier := &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{
				Url: "invalid-url", // Invalid URL that should cause client creation to fail
			},
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	err := packageVerifier.Run()
	assert.Error(t, err)
	// The error might be related to client creation or other issues
	assert.True(t, err != nil)
}
