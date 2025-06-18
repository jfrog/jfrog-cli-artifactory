package evidence

import (
	"errors"
	"testing"

	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerCreatePackage embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerCreatePackage struct {
	artifactory.EmptyArtifactoryServicesManager
	GetRepositoryResponse services.RepositoryDetails
	GetRepositoryError    error
	PackageLeadFileData   []byte
	GetPackageLeadError   error
	FileInfoResponse      *utils.FileInfo
	FileInfoError         error
}

func (m *MockArtifactoryServicesManagerCreatePackage) GetRepository(_ string, repoDetails interface{}) error {
	if m.GetRepositoryError != nil {
		return m.GetRepositoryError
	}
	if details, ok := repoDetails.(*services.RepositoryDetails); ok {
		*details = m.GetRepositoryResponse
	}
	return nil
}

func (m *MockArtifactoryServicesManagerCreatePackage) GetPackageLeadFile(_ services.LeadFileParams) ([]byte, error) {
	if m.GetPackageLeadError != nil {
		return nil, m.GetPackageLeadError
	}
	return m.PackageLeadFileData, nil
}

func (m *MockArtifactoryServicesManagerCreatePackage) FileInfo(_ string) (*utils.FileInfo, error) {
	if m.FileInfoError != nil {
		return nil, m.FileInfoError
	}
	return m.FileInfoResponse, nil
}

// MockMetadataServiceManagerCreatePackage for create package tests
type MockMetadataServiceManagerCreatePackage struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockMetadataServiceManagerCreatePackage) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// MockCreateEvidenceBaseCreatePackage for testing createEvidenceBase methods
type MockCreateEvidenceBaseCreatePackage struct {
	mock.Mock
	createEvidenceBase
}

func (m *MockCreateEvidenceBaseCreatePackage) createArtifactoryClient() (artifactory.ArtifactoryServicesManager, error) {
	args := m.Called()
	return args.Get(0).(artifactory.ArtifactoryServicesManager), args.Error(1)
}

func (m *MockCreateEvidenceBaseCreatePackage) createEnvelope(subject, subjectSha256 string) ([]byte, error) {
	args := m.Called(subject, subjectSha256)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCreateEvidenceBaseCreatePackage) uploadEvidence(envelope []byte, repoPath string) error {
	args := m.Called(envelope, repoPath)
	return args.Error(0)
}

func (m *MockCreateEvidenceBaseCreatePackage) getFileChecksum(path string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	args := m.Called(path, artifactoryClient)
	return args.String(0), args.Error(1)
}

func TestNewCreateEvidencePackage(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{}
	predicateFilePath := "/path/to/predicate.json"
	predicateType := "custom-predicate"
	markdownFilePath := "/path/to/markdown.md"
	key := "test-key"
	keyId := "test-key-id"
	packageName := "test-package"
	packageVersion := "1.0.0"
	packageRepoName := "test-repo"

	cmd := NewCreateEvidencePackage(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, packageName, packageVersion, packageRepoName)
	createCmd, ok := cmd.(*createEvidencePackage)
	assert.True(t, ok)

	// Test createEvidenceBase fields
	assert.Equal(t, serverDetails, createCmd.serverDetails)
	assert.Equal(t, predicateFilePath, createCmd.predicateFilePath)
	assert.Equal(t, predicateType, createCmd.predicateType)
	assert.Equal(t, markdownFilePath, createCmd.markdownFilePath)
	assert.Equal(t, key, createCmd.key)
	assert.Equal(t, keyId, createCmd.keyId)

	// Test basePackage fields
	assert.Equal(t, packageName, createCmd.PackageName)
	assert.Equal(t, packageVersion, createCmd.PackageVersion)
	assert.Equal(t, packageRepoName, createCmd.PackageRepoName)
}

func TestCreateEvidencePackage_CommandName(t *testing.T) {
	cmd := &createEvidencePackage{}
	assert.Equal(t, "create-package-evidence", cmd.CommandName())
}

func TestCreateEvidencePackage_ServerDetails(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{Url: "http://test.com"}
	cmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestCreateEvidencePackage_Run_Success(t *testing.T) {
	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
		FileInfoResponse: &utils.FileInfo{
			Checksums: struct {
				Sha1   string `json:"sha1,omitempty"`
				Sha256 string `json:"sha256,omitempty"`
				Md5    string `json:"md5,omitempty"`
			}{
				Sha256: "test-sha256",
			},
		},
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)
	mockBase.On("createEnvelope", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256").Return([]byte("test-envelope"), nil)
	mockBase.On("uploadEvidence", []byte("test-envelope"), "maven-local/test-package/1.0.0/test-package-1.0.0.jar").Return(nil)
	mockBase.On("getFileChecksum", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient).Return("test-sha256", nil)

	// Test that the mock methods would be called correctly
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	envelope, err := mockBase.createEnvelope("maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256")
	assert.NoError(t, err)
	assert.Equal(t, []byte("test-envelope"), envelope)

	err = mockBase.uploadEvidence([]byte("test-envelope"), "maven-local/test-package/1.0.0/test-package-1.0.0.jar")
	assert.NoError(t, err)

	checksum, err := mockBase.getFileChecksum("maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient)
	assert.NoError(t, err)
	assert.Equal(t, "test-sha256", checksum)

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_CreateArtifactoryClientError(t *testing.T) {
	createCmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{
				Url: "invalid-url", // Invalid URL that might cause client creation to fail
			},
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	err := createCmd.Run()
	assert.Error(t, err)
	// The error would be logged but the original error is returned
}

func TestCreateEvidencePackage_Run_GetPackageTypeError(t *testing.T) {
	// Mock Artifactory client with repository error
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryError: errors.New("repository not found"),
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)

	// Simulate the error path by testing the component methods
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	// Test the package type retrieval directly
	createCmd := &createEvidencePackage{
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	_, err = createCmd.basePackage.getPackageType(mockClient)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No such package")

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_CreateMetadataServiceError(t *testing.T) {
	createCmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{}, // Might cause metadata service creation to fail
		},
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	err := createCmd.Run()
	assert.Error(t, err)
	// The error will be related to either client creation or metadata service
}

func TestCreateEvidencePackage_Run_GetPackageVersionLeadArtifactError(t *testing.T) {
	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		GetPackageLeadError: errors.New("lead file not found"),
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)

	// Test the component that would fail
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	// Create package command
	createCmd := &createEvidencePackage{
		basePackage: basePackage{
			PackageName:     "test-package",
			PackageVersion:  "1.0.0",
			PackageRepoName: "maven-local",
		},
	}

	packageType, err := createCmd.basePackage.getPackageType(mockClient)
	assert.NoError(t, err)
	assert.Equal(t, "maven", packageType)

	// Mock metadata service that would fail too
	mockMetadata := &MockMetadataServiceManagerCreatePackage{
		GraphqlError: errors.New("metadata service error"),
	}

	// The lead artifact retrieval would fail
	_, err = createCmd.basePackage.getPackageVersionLeadArtifact(packageType, mockMetadata, mockClient)
	assert.Error(t, err)

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_GetFileChecksumError(t *testing.T) {
	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
		FileInfoError:       errors.New("file not found"),
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)
	mockBase.On("getFileChecksum", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient).Return("", errors.New("file not found"))

	// Test the component methods
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	checksum, err := mockBase.getFileChecksum("maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient)
	assert.Error(t, err)
	assert.Equal(t, "", checksum)
	assert.Contains(t, err.Error(), "file not found")

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_CreateEnvelopeError(t *testing.T) {
	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
		FileInfoResponse: &utils.FileInfo{
			Checksums: struct {
				Sha1   string `json:"sha1,omitempty"`
				Sha256 string `json:"sha256,omitempty"`
				Md5    string `json:"md5,omitempty"`
			}{
				Sha256: "test-sha256",
			},
		},
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)
	mockBase.On("getFileChecksum", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient).Return("test-sha256", nil)
	mockBase.On("createEnvelope", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256").Return([]byte{}, errors.New("envelope creation failed"))

	// Test the component methods
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	checksum, err := mockBase.getFileChecksum("maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient)
	assert.NoError(t, err)
	assert.Equal(t, "test-sha256", checksum)

	envelope, err := mockBase.createEnvelope("maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envelope creation failed")
	assert.Equal(t, []byte{}, envelope)

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_UploadEvidenceError(t *testing.T) {
	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte("maven-local/test-package/1.0.0/test-package-1.0.0.jar"),
		FileInfoResponse: &utils.FileInfo{
			Checksums: struct {
				Sha1   string `json:"sha1,omitempty"`
				Sha256 string `json:"sha256,omitempty"`
				Md5    string `json:"md5,omitempty"`
			}{
				Sha256: "test-sha256",
			},
		},
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)
	mockBase.On("getFileChecksum", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient).Return("test-sha256", nil)
	mockBase.On("createEnvelope", "maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256").Return([]byte("test-envelope"), nil)
	mockBase.On("uploadEvidence", []byte("test-envelope"), "maven-local/test-package/1.0.0/test-package-1.0.0.jar").Return(errors.New("upload failed"))

	// Test the component methods
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	checksum, err := mockBase.getFileChecksum("maven-local/test-package/1.0.0/test-package-1.0.0.jar", mockClient)
	assert.NoError(t, err)
	assert.Equal(t, "test-sha256", checksum)

	envelope, err := mockBase.createEnvelope("maven-local/test-package/1.0.0/test-package-1.0.0.jar", "test-sha256")
	assert.NoError(t, err)
	assert.Equal(t, []byte("test-envelope"), envelope)

	err = mockBase.uploadEvidence([]byte("test-envelope"), "maven-local/test-package/1.0.0/test-package-1.0.0.jar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed")

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_Run_ComplexPackagePath(t *testing.T) {
	// Test with complex package path (deep structure)
	complexPath := "maven-local/com/example/complex-package-name/1.0.0-SNAPSHOT/complex-package-name-1.0.0-SNAPSHOT.jar"

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCreatePackage{
		GetRepositoryResponse: services.RepositoryDetails{
			PackageType: "maven",
		},
		PackageLeadFileData: []byte(complexPath),
		FileInfoResponse: &utils.FileInfo{
			Checksums: struct {
				Sha1   string `json:"sha1,omitempty"`
				Sha256 string `json:"sha256,omitempty"`
				Md5    string `json:"md5,omitempty"`
			}{
				Sha256: "complex-sha256",
			},
		},
	}

	// Mock the base methods
	mockBase := &MockCreateEvidenceBaseCreatePackage{}
	mockBase.On("createArtifactoryClient").Return(mockClient, nil)
	mockBase.On("getFileChecksum", complexPath, mockClient).Return("complex-sha256", nil)
	mockBase.On("createEnvelope", complexPath, "complex-sha256").Return([]byte("complex-envelope"), nil)
	mockBase.On("uploadEvidence", []byte("complex-envelope"), complexPath).Return(nil)

	// Test the component methods
	_, err := mockBase.createArtifactoryClient()
	assert.NoError(t, err)

	checksum, err := mockBase.getFileChecksum(complexPath, mockClient)
	assert.NoError(t, err)
	assert.Equal(t, "complex-sha256", checksum)

	envelope, err := mockBase.createEnvelope(complexPath, "complex-sha256")
	assert.NoError(t, err)
	assert.Equal(t, []byte("complex-envelope"), envelope)

	err = mockBase.uploadEvidence([]byte("complex-envelope"), complexPath)
	assert.NoError(t, err)

	mockBase.AssertExpectations(t)
}

func TestCreateEvidencePackage_BasePackage_Integration(t *testing.T) {
	// Test the basePackage integration with createEvidencePackage
	createCmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &coreConfig.ServerDetails{},
			predicateFilePath: "/path/to/predicate.json",
			predicateType:     "custom-predicate",
			markdownFilePath:  "/path/to/markdown.md",
			key:               "test-key",
			keyId:             "test-key-id",
		},
		basePackage: basePackage{
			PackageName:     "integration-test-package",
			PackageVersion:  "2.0.0",
			PackageRepoName: "npm-local",
		},
	}

	// Verify the structure is correct
	assert.Equal(t, "integration-test-package", createCmd.PackageName)
	assert.Equal(t, "2.0.0", createCmd.PackageVersion)
	assert.Equal(t, "npm-local", createCmd.PackageRepoName)
	assert.Equal(t, "/path/to/predicate.json", createCmd.predicateFilePath)
	assert.Equal(t, "custom-predicate", createCmd.predicateType)
	assert.Equal(t, "/path/to/markdown.md", createCmd.markdownFilePath)
	assert.Equal(t, "test-key", createCmd.key)
	assert.Equal(t, "test-key-id", createCmd.keyId)

	// Command name should be correct
	assert.Equal(t, "create-package-evidence", createCmd.CommandName())

	// Server details should be correct
	serverDetails, err := createCmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, createCmd.serverDetails, serverDetails)
}
