package evidence

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"

	"github.com/stretchr/testify/assert"
)

// MockPackageService implements PackageService interface for testing
type MockPackageService struct {
	PackageName     string
	PackageVersion  string
	PackageRepoName string
	PackageType     string
	LeadArtifact    string
	ShouldError     bool
	ErrorMsg        string
}

func (m *MockPackageService) GetPackageType(_ artifactory.ArtifactoryServicesManager) (string, error) {
	if m.ShouldError {
		return "", fmt.Errorf(m.ErrorMsg)
	}
	return m.PackageType, nil
}

func (m *MockPackageService) GetPackageVersionLeadArtifact(_ string, _ metadata.Manager, _ artifactory.ArtifactoryServicesManager) (string, error) {
	if m.ShouldError {
		return "", fmt.Errorf(m.ErrorMsg)
	}
	return m.LeadArtifact, nil
}

func (m *MockPackageService) GetPackageName() string {
	return m.PackageName
}

func (m *MockPackageService) GetPackageVersion() string {
	return m.PackageVersion
}

func (m *MockPackageService) GetPackageRepoName() string {
	return m.PackageRepoName
}

type mockMetadataServiceManagerDuplicateRepositories struct{}

func (m *mockMetadataServiceManagerDuplicateRepositories) GraphqlQuery(_ []byte) ([]byte, error) {
	response := `{"data":{"versions":{"edges":[{"node":{"repos":[{"name":"nuget-local","leadFilePath":"MyLibrary/1.0.0/test.1.0.0.nupkg"},{"name":"local-test","leadFilePath":"MyLibrary/1.0.0/test.1.0.0.nupkg"}]}]}}}}`
	return []byte(response), nil
}

type mockMetadataServiceManagerGoodResponse struct{}

func (m *mockMetadataServiceManagerGoodResponse) GraphqlQuery(_ []byte) ([]byte, error) {
	response := `{"data":{"versions":{"edges":[{"node":{"repos":[{"name":"nuget-local","leadFilePath":"MyLibrary/1.0.0/test.1.0.0.nupkg"}]}}]}}}`
	return []byte(response), nil
}

type mockMetadataServiceManagerBadResponse struct{}

func (m *mockMetadataServiceManagerBadResponse) GraphqlQuery(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("HTTP %d: Not Found", http.StatusNotFound)
}

type mockArtifactoryServicesManagerGoodResponse struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManagerGoodResponse) GetPackageLeadFile(services.LeadFileParams) ([]byte, error) {
	return []byte("docker-local/MyLibrary/1.0.0/test.1.0.0.docker"), nil
}

type mockArtifactoryServicesManagerBadResponse struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManagerBadResponse) GetPackageLeadFile(services.LeadFileParams) ([]byte, error) {
	return nil, fmt.Errorf("HTTP %d: Not Found", http.StatusNotFound)
}

func TestGetLeadFileFromMetadataService(t *testing.T) {
	tests := []struct {
		name                     string
		metadataClientMock       metadata.Manager
		artifactoryClientMock    *mockArtifactoryServicesManagerBadResponse
		packageName              string
		packageVersion           string
		repoName                 string
		packageType              string
		expectedLeadArtifactPath string
		expectError              bool
	}{
		{
			name:                     "Get lead artifact successfully from metadata service",
			metadataClientMock:       &mockMetadataServiceManagerGoodResponse{},
			artifactoryClientMock:    &mockArtifactoryServicesManagerBadResponse{},
			packageName:              "test",
			packageVersion:           "1.0.0",
			repoName:                 "nuget-local",
			packageType:              "nuget",
			expectedLeadArtifactPath: "nuget-local/MyLibrary/1.0.0/test.1.0.0.nupkg",
			expectError:              false,
		},
		{
			name:                     "Duplicate package name and version in the same repository",
			metadataClientMock:       &mockMetadataServiceManagerDuplicateRepositories{},
			artifactoryClientMock:    &mockArtifactoryServicesManagerBadResponse{},
			packageName:              "test",
			packageVersion:           "1.0.0",
			repoName:                 "nuget-local",
			packageType:              "nuget",
			expectedLeadArtifactPath: "",
			expectError:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packageService := NewPackageService(tt.packageName, tt.packageVersion, tt.repoName)
			leadArtifactPath, err := packageService.GetPackageVersionLeadArtifact(tt.packageType, tt.metadataClientMock, tt.artifactoryClientMock)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, leadArtifactPath)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLeadArtifactPath, leadArtifactPath)
			}
		})
	}
}

func TestGetLeadArtifactFromArtifactoryServiceSuccess(t *testing.T) {
	metadataClientMock := &mockMetadataServiceManagerGoodResponse{}
	artifactoryClientMock := &mockArtifactoryServicesManagerGoodResponse{}

	packageService := NewPackageService("test", "1.0.0", "nuget-local")

	leadArtifactPath, err := packageService.GetPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.NoError(t, err)
	assert.Equal(t, "docker-local/MyLibrary/1.0.0/test.1.0.0.docker", leadArtifactPath)
}

func TestGetLeadFileFromArtifactFailsFromMetadataSuccess(t *testing.T) {
	metadataClientMock := &mockMetadataServiceManagerGoodResponse{}
	artifactoryClientMock := &mockArtifactoryServicesManagerBadResponse{}

	packageService := NewPackageService("test", "1.0.0", "nuget-local")

	leadArtifactPath, err := packageService.GetPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.NoError(t, err)
	assert.Equal(t, "nuget-local/MyLibrary/1.0.0/test.1.0.0.nupkg", leadArtifactPath)
}

func TestGetLeadArtifactFailsBothServices(t *testing.T) {
	metadataClientMock := &mockMetadataServiceManagerBadResponse{}
	artifactoryClientMock := &mockArtifactoryServicesManagerBadResponse{}

	packageService := NewPackageService("test", "1.0.0", "nuget-local")

	leadArtifactPath, err := packageService.GetPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.Error(t, err)
	assert.Empty(t, leadArtifactPath)
}

// customMockArtifactoryServicesManager is a test double for ArtifactoryServicesManager for TestReplaceFirstColon
// It returns the provided input as the lead file.
type customMockArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	leadFile []byte
}

func (m *customMockArtifactoryServicesManager) GetPackageLeadFile(_ services.LeadFileParams) ([]byte, error) {
	return m.leadFile, nil
}

func TestReplaceFirstColon(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "Replace first colon",
			input:    []byte("sha256:fsndlknlkqnlfnqksd"),
			expected: "sha256/fsndlknlkqnlfnqksd",
		},
		{
			name:     "No colon to replace",
			input:    []byte("sha256-fsndlknlkqnlfnqksd"),
			expected: "sha256-fsndlknlkqnlfnqksd",
		},
		{
			name:     "Multiple colons",
			input:    []byte("repo:sha256:fsndlknlkqnlfnqksd"),
			expected: "repo/sha256:fsndlknlkqnlfnqksd",
		},
		{
			name:     "Colon at the beginning",
			input:    []byte(":sha256:fsndlknlkqnlfnqksd"),
			expected: "/sha256:fsndlknlkqnlfnqksd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataClientMock := &mockMetadataServiceManagerBadResponse{}
			artifactoryClientMock := &customMockArtifactoryServicesManager{leadFile: tt.input}
			packageService := NewPackageService("test", "1.0.0", "nuget-local")
			leadArtifactPath, err := packageService.GetPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, leadArtifactPath)
		})
	}
}

func TestPackageServiceInterface(t *testing.T) {
	// Test that basePackage implements PackageService interface
	var _ PackageService = (*basePackage)(nil)

	// Test that MockPackageService implements PackageService interface
	var _ PackageService = (*MockPackageService)(nil)
}

func TestNewPackageService(t *testing.T) {
	name := "test-package"
	version := "1.0.0"
	repoName := "test-repo"

	ps := NewPackageService(name, version, repoName)

	assert.Equal(t, name, ps.GetPackageName())
	assert.Equal(t, version, ps.GetPackageVersion())
	assert.Equal(t, repoName, ps.GetPackageRepoName())

	// Verify it's the correct type
	_, ok := ps.(*basePackage)
	assert.True(t, ok)
}

func TestPackageServiceWithMock(t *testing.T) {
	mockPS := &MockPackageService{
		PackageName:     "mock-package",
		PackageVersion:  "2.0.0",
		PackageRepoName: "mock-repo",
		PackageType:     "nuget",
		LeadArtifact:    "mock-repo/mock-package/2.0.0/package.nupkg",
	}

	assert.Equal(t, "mock-package", mockPS.GetPackageName())
	assert.Equal(t, "2.0.0", mockPS.GetPackageVersion())
	assert.Equal(t, "mock-repo", mockPS.GetPackageRepoName())

	// Test error case
	mockPS.ShouldError = true
	mockPS.ErrorMsg = "test error"

	_, err := mockPS.GetPackageType(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
}
