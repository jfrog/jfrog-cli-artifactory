package evidence

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
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
	cmd := NewGetEvidenceReleaseBundle(serverDetails, "myBundle", "1.0", "myProject", "json", "output.json", true)

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

func (g *getEvidenceReleaseBundle) createOnemodelServiceManager(serverDetails *config.ServerDetails, includePredicate bool) (onemodel.Manager, error) {
	return utils.CreateOnemodelServiceManager(serverDetails, includePredicate)
}

func TestCreateReleaseBundleGetEvidenceQuery(t *testing.T) {
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

	query := g.createGetEvidenceQuery("myBundle", "1.0")
	expectedQuery := fmt.Sprintf(`{"query": "{\n  releaseBundleVersion {\n    getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {\n      createdBy\n      createdAt\n      evidenceConnection {\n        edges {\n          cursor\n          node {\n            path\n            name\n            predicateSlug\n          }\n        }\n      }\n      artifactsConnection(first: 1000, after:\"YXJ0aWZhY3Q6MA==\", where:{\n  hasEvidence: true\n}) {\n        totalCount\n        pageInfo {\n          hasNextPage\n          hasPreviousPage\n          startCursor\n          endCursor\n        }\n        edges {\n          cursor\n          node {\n            path\n            name\n            packageType\n            sourceRepositoryPath\n            evidenceConnection(first: 0) {\n              totalCount\n              pageInfo {\n                hasNextPage\n                hasPreviousPage\n                startCursor\n                endCursor\n              }\n              edges {\n                cursor\n                node {\n                  path\n                  name\n                  predicateSlug\n                }\n              }\n            }\n          }\n        }\n      }\n      fromBuilds {\n        name\n        number\n        startedAt\n        evidenceConnection {\n          edges {\n            node {\n              path\n              name\n              predicateSlug\n            }\n          }\n        }\n      }\n    }\n  }\n}"}`, "myProject-release-bundles-v2", "myBundle", "1.0")

	assert.Equal(t, string(query), expectedQuery)
}
