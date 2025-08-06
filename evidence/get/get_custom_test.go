package get

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
	response := `{"data":{"evidence":{"searchEvidence":{"totalCount":1,"edges":[{"cursor":"1","node":{"predicateSlug":"test-slug","downloadPath":"test/path","verified":true,"signingKey":{"alias":"test-alias"},"subject":{"sha256":"test-digest"},"createdBy":"test-user","createdAt":"2024-01-01T00:00:00Z"}}]}}}}`
	return []byte(response), nil
}

// Mock of the Onemodel Manager for an error scenario
type mockOnemodelManagerCustomError struct{}

func (m *mockOnemodelManagerCustomError) GraphqlQuery(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("HTTP %d: Not Found", http.StatusNotFound)
}

func validatePredicateEvidence(t *testing.T, result []byte) {
	var output CustomEvidenceOutput
	err := json.Unmarshal(result, &output)
	assert.NoError(t, err)

	assert.Equal(t, SchemaVersion, output.SchemaVersion)
	assert.Equal(t, ArtifactType, output.Type)
	assert.Equal(t, "test-repo/path/file.json", output.Result.RepoPath)
	assert.Len(t, output.Result.Evidence, 1)

	firstEvidence := output.Result.Evidence[0]
	assert.Equal(t, "user@example.com", firstEvidence.CreatedBy)
	assert.Equal(t, "distribution-v1", firstEvidence.PredicateSlug)
	assert.Equal(t, map[string]any{"analysis": "sbom"}, firstEvidence.Predicate)
	assert.Equal(t, true, firstEvidence.Verified)
}

func validateEmptyEvidence(t *testing.T, result []byte) {
	var output CustomEvidenceOutput
	err := json.Unmarshal(result, &output)
	assert.NoError(t, err)

	assert.Equal(t, SchemaVersion, output.SchemaVersion)
	assert.Equal(t, ArtifactType, output.Type)
	assert.Equal(t, "test-repo/path/file.txt", output.Result.RepoPath)
	assert.Len(t, output.Result.Evidence, 0)
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
				var data map[string]any
				if err := json.Unmarshal(evidence, &data); err == nil {
					if evidenceData, ok := data["data"].(map[string]any); ok {
						if evidenceNode, ok := evidenceData["evidence"].(map[string]any); ok {
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
		name          string
		fullPath      string
		expectedRepo  string
		expectedPath  string
		expectedName  string
		expectedError bool
	}{
		{
			name:          "Full path with multiple directories",
			fullPath:      "repo-key/my/path/to/file/file.txt",
			expectedRepo:  "repo-key",
			expectedPath:  "my/path/to/file",
			expectedName:  "file.txt",
			expectedError: false,
		},
		{
			name:          "Path with a file directly in the repo",
			fullPath:      "another-repo/image.jpg",
			expectedRepo:  "another-repo",
			expectedPath:  "",
			expectedName:  "image.jpg",
			expectedError: false,
		},
		{
			name:          "Path with two levels",
			fullPath:      "myRepo/my/path",
			expectedRepo:  "myRepo",
			expectedPath:  "my",
			expectedName:  "path",
			expectedError: false,
		},
		{
			name:          "Invalid input with no slash",
			fullPath:      "invalidFormat",
			expectedRepo:  "",
			expectedPath:  "",
			expectedName:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceCustom{}
			repo, path, name, err := g.getRepoKeyAndPath(tt.fullPath)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, repo)
				assert.Empty(t, path)
				assert.Empty(t, name)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRepo, repo)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}

func TestTransformGraphQLOutput(t *testing.T) {
	tests := []struct {
		name            string
		subjectRepoPath string
		inputFile       string
		expectedFile    string
		expectedError   bool
		errorContains   string
		validateFunc    func(t *testing.T, result []byte)
	}{
		{
			name:            "Multiple evidence entries",
			subjectRepoPath: "dort-generic/test/path/file.txt",
			inputFile:       "multiple_evidence_input.json",
			expectedFile:    "multiple_evidence_expected.json",
			expectedError:   false,
		},
		{
			name:            "Evidence with predicate field",
			subjectRepoPath: "test-repo/path/file.json",
			inputFile:       "predicate_evidence_input.json",
			expectedError:   false,
			validateFunc:    validatePredicateEvidence,
		},
		{
			name:            "Empty evidence response",
			subjectRepoPath: "test-repo/path/file.txt",
			inputFile:       "empty_evidence_input.json",
			expectedError:   false,
			validateFunc:    validateEmptyEvidence,
		},
		{
			name:            "Invalid JSON",
			subjectRepoPath: "test-repo/path/file.txt",
			inputFile:       "",
			expectedError:   true,
		},
		{
			name:            "Missing data field",
			subjectRepoPath: "test-repo/path/file.txt",
			inputFile:       "",
			expectedError:   true,
			errorContains:   "missing data field",
		},
		{
			name:            "Missing evidence field",
			subjectRepoPath: "test-repo/path/file.txt",
			inputFile:       "",
			expectedError:   true,
			errorContains:   "missing evidence field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &getEvidenceCustom{
				subjectRepoPath: tt.subjectRepoPath,
				getEvidenceBase: getEvidenceBase{
					includePredicate: tt.name == "Evidence with predicate field",
				},
			}

			var input []byte
			var err error

			if tt.inputFile != "" {
				inputStr, err := ReadTestDataFile(tt.inputFile)
				if err != nil {
					t.Fatalf("Failed to read input file: %v", err)
				}
				input = []byte(inputStr)
			} else {
				switch tt.name {
				case "Invalid JSON":
					input = []byte("invalid json")
				case "Missing data field":
					input = []byte(`{"someOtherField": "value"}`)
				case "Missing evidence field":
					input = []byte(`{"data": {"someOtherField": "value"}}`)
				default:
					t.Fatalf("No input file specified and no inline case for: %s", tt.name)
				}
			}

			result, err := g.transformGraphQLOutput(input)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				if tt.expectedFile != "" {
					expectedStr, err := ReadTestDataFile(tt.expectedFile)
					if err != nil {
						t.Fatalf("Failed to read expected output file: %v", err)
					}
					var expected, actual map[string]any
					err = json.Unmarshal([]byte(expectedStr), &expected)
					assert.NoError(t, err)
					err = json.Unmarshal(result, &actual)
					assert.NoError(t, err)
					assert.Equal(t, expected, actual)
				} else if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}
