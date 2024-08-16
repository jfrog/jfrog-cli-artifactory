package evidence

import (
	"github.com/jfrog/jfrog-client-go/metadata"
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

func TestPackage(t *testing.T) {
	tests := []struct {
		name                     string
		metadataClientMock       metadata.Manager
		packageName              string
		packageVersion           string
		repoName                 string
		packageType              string
		expectedLeadArtifactPath string
		expectError              bool
	}{
		{
			name:                     "Get lead artifact successfully",
			metadataClientMock:       &mockMetadataServiceManagerGoodResponse{},
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
			leadArtifact, err := c.getPackageVersionLeadArtifact(tt.packageType, tt.metadataClientMock)
			leadArtifactPath := c.buildLeadArtifactPath(leadArtifact)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, leadArtifact)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLeadArtifactPath, leadArtifactPath)
			}
		})
	}
}
