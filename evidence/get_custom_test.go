package evidence

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/stretchr/testify/assert"
)

// Mock of the Onemodel Manager for a successful query
type mockOnemodelManagerCustomSuccess struct{}

func (m *mockOnemodelManagerCustomSuccess) GraphqlQuery(_ []byte) ([]byte, error) {
	response := `{"data":{"evidence":{"searchEvidence":{"totalCount":1,"edges":[{"cursor":"1","node":{"path":"path/to/evidence","name":"evidenceName","predicateSlug":"slug"}}]}}}}}`
	return []byte(response), nil
}

// Mock of the Onemodel Manager for an error scenario
type mockOnemodelManagerCustomError struct{}

func (m *mockOnemodelManagerCustomError) GraphqlQuery(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("HTTP %d: Not Found", http.StatusNotFound)
}

// TestNewGetEvidenceCustom
func TestNewGetEvidenceCustom(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	cmd := NewGetEvidenceCustom(serverDetails, "repo/path", "json", "output.json", true)

	// Verify it's of the expected type
	evidenceCustom, ok := cmd.(*getEvidenceCustom)
	assert.True(t, ok)
	assert.IsType(t, &getEvidenceCustom{}, evidenceCustom)
	assert.Equal(t, serverDetails, evidenceCustom.serverDetails)
	assert.Equal(t, "repo/path", evidenceCustom.subjectRepoPath)
	assert.Equal(t, "json", evidenceCustom.format)
	assert.Equal(t, "output.json", evidenceCustom.outputFileName)
	assert.True(t, evidenceCustom.includePredicate)
}

// Test getEvidence method
func TestGetCustomEvidence(t *testing.T) {
	tests := []struct {
		name                string
		onemodelClient      onemodel.Manager
		expectedError       bool
		expectedEvidenceLen int
	}{
		{
			name:                "Successful evidence retrieval",
			onemodelClient:      &mockOnemodelManagerCustomSuccess{},
			expectedError:       false,
			expectedEvidenceLen: 1,
		},
		{
			name:                "Error retrieving evidence",
			onemodelClient:      &mockOnemodelManagerCustomError{},
			expectedError:       true,
			expectedEvidenceLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceCustom{
				subjectRepoPath: "myRepo/my/path",
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

				// Additional check on the number of edges in the result
				var data map[string]interface{}
				if err := json.Unmarshal(evidence, &data); err == nil {
					if evidenceData, ok := data["data"].(map[string]interface{}); ok {
						if evidenceNode, ok := evidenceData["evidence"].(map[string]interface{}); ok {
							if searchEvidence, ok := evidenceNode["searchEvidence"].(map[string]any); ok {
								edgesInterface, ok := searchEvidence["edges"].([]any)
								if !ok {
									log.Fatalf("Type assertion failed: expected []any")
								}
								edges := edgesInterface
								assert.Equal(t, tt.expectedEvidenceLen, len(edges))
							}
						}
					}
				}
			}
		})
	}
}

// Test getRepoKeyAndPath method
func TestGetRepoKeyAndPath(t *testing.T) {
	tests := []struct {
		name            string
		subjectRepoPath string
		expectedRepo    string
		expectedPath    string
		expectedError   bool
	}{
		{
			name:            "Valid repo/path format",
			subjectRepoPath: "myRepo/my/path",
			expectedRepo:    "myRepo",
			expectedPath:    "my/path",
			expectedError:   false,
		},
		{
			name:            "Invalid repo/path format",
			subjectRepoPath: "invalidFormat",
			expectedRepo:    "",
			expectedPath:    "",
			expectedError:   true,
		},
		{
			name:            "Edge case with a single repo in the path",
			subjectRepoPath: "myRepo/",
			expectedRepo:    "myRepo",
			expectedPath:    "",
			expectedError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceCustom{}
			repo, path, err := g.getRepoKeyAndPath(tt.subjectRepoPath)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, repo)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRepo, repo)
				assert.Equal(t, tt.expectedPath, path)
			}
		})
	}
}
