package evidence

import (
	"fmt"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			c := &createEvidencePackage{
				packageName:     tt.packageName,
				packageVersion:  tt.packageVersion,
				packageRepoName: tt.repoName,
			}
			leadArtifactPath, err := c.getPackageVersionLeadArtifact(tt.packageType, tt.metadataClientMock, tt.artifactoryClientMock)

			if tt.expectError {
				assert.Nil(t, err)
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

	c := &createEvidencePackage{
		packageName:     "test",
		packageVersion:  "1.0.0",
		packageRepoName: "nuget-local",
	}

	leadArtifactPath, err := c.getPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.NoError(t, err)
	assert.Equal(t, "docker-local/MyLibrary/1.0.0/test.1.0.0.docker", leadArtifactPath)
}

func TestGetLeadFileFromArtifactFailsFromMetadataSuccess(t *testing.T) {
	metadataClientMock := &mockMetadataServiceManagerGoodResponse{}
	artifactoryClientMock := &mockArtifactoryServicesManagerBadResponse{}

	c := &createEvidencePackage{
		packageName:     "test",
		packageVersion:  "1.0.0",
		packageRepoName: "nuget-local",
	}

	leadArtifactPath, err := c.getPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.NoError(t, err)
	assert.Equal(t, "nuget-local/MyLibrary/1.0.0/test.1.0.0.nupkg", leadArtifactPath)
}

func TestGetLeadArtifactFailsBothServices(t *testing.T) {
	metadataClientMock := &mockMetadataServiceManagerBadResponse{}
	artifactoryClientMock := &mockArtifactoryServicesManagerBadResponse{}

	c := &createEvidencePackage{
		packageName:     "test",
		packageVersion:  "1.0.0",
		packageRepoName: "nuget-local",
	}

	leadArtifactPath, err := c.getPackageVersionLeadArtifact("nuget", metadataClientMock, artifactoryClientMock)

	assert.Nil(t, err)
	assert.Empty(t, leadArtifactPath)
}
