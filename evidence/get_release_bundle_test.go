package evidence

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/stretchr/testify/assert"
)

// Mock of the Onemodel Manager for successful query execution
type mockOnemodelManagerSuccess struct{}

func (m *mockOnemodelManagerSuccess) GraphqlQuery(_ []byte) ([]byte, error) {
	response := `{"data":{"releaseBundleVersion":{"getVersion":{"createdBy":"user","createdAt":"2021-01-01T00:00:00Z","evidenceConnection":{"edges":[{"cursor":"1","node":{"path":"path/to/evidence","name":"evidenceName","predicateSlug":"slug"}}]},"artifactsConnection":{"totalCount":1,"edges":[{"cursor":"artifact1","node":{"path":"path/to/artifact","name":"artifactName","packageType":"npm","sourceRepositoryPath":"npm-local","evidenceConnection":{"totalCount":1}}}]}}}}}`
	return []byte(response), nil
}

// Mock of the Onemodel Manager for error handling
type mockOnemodelManagerError struct{}

func (m *mockOnemodelManagerError) GraphqlQuery(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("HTTP %d: Not Found", http.StatusNotFound)
}

func TestNewGetEvidenceReleaseBundle(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	cmd := NewGetEvidenceReleaseBundle(serverDetails, "myBundle", "1.0", "myProject", "json", "output.json", "1000", true)

	bundle, ok := cmd.(*getEvidenceReleaseBundle)

	assert.True(t, ok)
	assert.IsType(t, &getEvidenceReleaseBundle{}, bundle)
	assert.Equal(t, serverDetails, bundle.serverDetails)
	assert.Equal(t, "myBundle", bundle.releaseBundle)
	assert.Equal(t, "1.0", bundle.releaseBundleVersion)
	assert.Equal(t, "myProject", bundle.project)
	assert.Equal(t, "json", bundle.format)
	assert.Equal(t, "output.json", bundle.outputFileName)
	assert.True(t, bundle.includePredicate)
}

func TestGetEvidence(t *testing.T) {
	tests := []struct {
		name             string
		onemodelClient   onemodel.Manager
		expectedError    bool
		expectedEvidence string
	}{
		{
			name:           "Successful evidence retrieval",
			onemodelClient: &mockOnemodelManagerSuccess{},
			expectedError:  false,
		},
		{
			name:           "Error retrieving evidence",
			onemodelClient: &mockOnemodelManagerError{},
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceReleaseBundle{
				releaseBundle:        "myBundle",
				releaseBundleVersion: "1.0",
				project:              "myProject",
				getEvidenceBase: getEvidenceBase{
					serverDetails:    &config.ServerDetails{},
					outputFileName:   "output.json",
					format:           "json",
					includePredicate: true,
				},
			}

			evidence, err := g.getEvidence(tt.onemodelClient)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, evidence)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, evidence)
			}
		})
	}
}

func TestCreateReleaseBundleGetEvidenceQuery(t *testing.T) {
	tests := []struct {
		name                 string
		project              string
		releaseBundle        string
		releaseBundleVersion string
		artifactsLimit       string
		includePredicate     bool
		expectedSubstring    string // We will check for a substring since the full query can be long
	}{
		{
			name:                 "Test with default project",
			project:              "",
			releaseBundle:        "bundle-1",
			releaseBundleVersion: "1.0",
			artifactsLimit:       "5",
			expectedSubstring:    "evidenceConnection",
		},
		{
			name:                 "Test with specific project",
			project:              "myProject",
			releaseBundle:        "bundle-2",
			releaseBundleVersion: "2.0",
			artifactsLimit:       "10",
			expectedSubstring:    "predicateSlug",
		},
		{
			name:                 "Test with empty artifacts limit, expects default limit",
			project:              "customProject",
			releaseBundle:        "bundle-3",
			releaseBundleVersion: "3.0",
			artifactsLimit:       "",
			expectedSubstring:    "evidenceConnection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceReleaseBundle{
				project:              tt.project,
				releaseBundle:        tt.releaseBundle,
				releaseBundleVersion: tt.releaseBundleVersion,
				artifactsLimit:       tt.artifactsLimit,
			}

			result := g.buildGraphqlQuery(tt.releaseBundle, tt.releaseBundleVersion)
			assert.Contains(t, string(result), tt.expectedSubstring)
		})
	}
}
