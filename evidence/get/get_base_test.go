package get

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportEvidenceToJsonlFileWithMetadata(t *testing.T) {
	tests := []struct {
		name           string
		input          any
		expectedLines  int
		expectedSchema string
		expectedType   string
	}{
		{
			name: "Custom evidence output with evidence array",
			input: CustomEvidenceOutput{
				SchemaVersion: SchemaVersion,
				Type:          ArtifactType,
				Result: CustomEvidenceResult{
					RepoPath: "test-repo/path/file.txt",
					Evidence: []EvidenceEntry{
						{CreatedBy: "user1", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil, CreatedAt: ""},
						{CreatedBy: "user2", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil, CreatedAt: ""},
					},
				},
			},
			expectedLines:  2,
			expectedSchema: SchemaVersion,
			expectedType:   "artifact",
		},
		{
			name: "Evidence output with no arrays",
			input: CustomEvidenceOutput{
				SchemaVersion: SchemaVersion,
				Type:          ArtifactType,
				Result: CustomEvidenceResult{
					RepoPath: "test-repo/path/file.txt",
					Evidence: []EvidenceEntry{},
				},
			},
			expectedLines:  0, // No evidence entries, so expect 0 lines
			expectedSchema: SchemaVersion,
			expectedType:   "artifact",
		},
		{
			name: "Release bundle with flattened evidence",
			input: ReleaseBundleOutput{
				SchemaVersion: SchemaVersion,
				Type:          ReleaseBundleType,
				Result: ReleaseBundleResult{
					ReleaseBundle:        "test-bundle",
					ReleaseBundleVersion: "1.0.0",
					Evidence: []EvidenceEntry{
						{CreatedBy: "user1", CreatedAt: "2024-12-02T07:18:43.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
						{CreatedBy: "user2", CreatedAt: "2024-12-02T07:18:44.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
					},
					Artifacts: []ArtifactEvidence{
						{
							Evidence:    EvidenceEntry{CreatedBy: "user3", CreatedAt: "2024-12-02T07:18:45.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
							PackageType: "docker",
							RepoPath:    "test-repo/artifact1",
						},
						{
							Evidence:    EvidenceEntry{CreatedBy: "user4", CreatedAt: "2024-12-02T07:18:46.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
							PackageType: "docker",
							RepoPath:    "test-repo/artifact1",
						},
					},
					Builds: []BuildEvidence{
						{
							Evidence:    EvidenceEntry{CreatedBy: "user5", CreatedAt: "2024-12-02T07:18:47.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
							BuildName:   "test-build",
							BuildNumber: "123",
							StartedAt:   "2024-12-02T07:17:00.000Z",
						},
						{
							Evidence:    EvidenceEntry{CreatedBy: "user6", CreatedAt: "2024-12-02T07:18:48.942Z", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil},
							BuildName:   "test-build",
							BuildNumber: "123",
							StartedAt:   "2024-12-02T07:17:00.000Z",
						},
					},
				},
			},
			expectedLines:  6, // 2 release-bundle + 2 artifact + 2 build evidence entries
			expectedSchema: SchemaVersion,
			expectedType:   "release-bundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(tt.input)
			assert.NoError(t, err)

			tempDir := t.TempDir()
			outputFile := path.Join(tempDir, "test_metadata.jsonl")

			err = exportEvidenceToJsonlFile(inputJSON, outputFile)
			assert.NoError(t, err)

			outputData, err := os.ReadFile(outputFile)
			assert.NoError(t, err)

			lines := strings.Split(strings.TrimSpace(string(outputData)), "\n")
			if tt.expectedLines == 0 {
				assert.True(t, len(lines) == 0 || (len(lines) == 1 && lines[0] == ""), "No lines should be written for empty evidence")
				return
			}
			assert.Len(t, lines, tt.expectedLines)

			for i, line := range lines {
				var item map[string]any
				err := json.Unmarshal([]byte(line), &item)
				assert.NoError(t, err, "Line %d should be valid JSON", i+1)

				assert.Contains(t, item, "schemaVersion", "Line %d should contain schemaVersion", i+1)
				assert.Contains(t, item, "type", "Line %d should contain type", i+1)
				assert.Contains(t, item, "result", "Line %d should contain result", i+1)

				assert.Equal(t, tt.expectedSchema, item["schemaVersion"], "Line %d should have correct schemaVersion", i+1)

				// For release bundle with flattened evidence, check that types are correct
				if tt.name == "Release bundle with flattened evidence" {
					if result, ok := item["result"].(map[string]any); ok {
						switch {
						case i < 2:
							// First two lines should be release-bundle type
							assert.Equal(t, "release-bundle", item["type"], "Line %d should be release-bundle type", i+1)
						case i < 4:
							// Next two lines should be artifact type
							assert.Equal(t, "artifact", item["type"], "Line %d should be artifact type", i+1)
							assert.Contains(t, result, "package-type", "Line %d should contain package-type", i+1)
							assert.Contains(t, result, "repo-path", "Line %d should contain repo-path", i+1)
						default:
							// Last two lines should be build type
							assert.Equal(t, "build", item["type"], "Line %d should be build type", i+1)
							assert.Contains(t, result, "build-name", "Line %d should contain build-name", i+1)
							assert.Contains(t, result, "build-number", "Line %d should contain build-number", i+1)
							assert.Contains(t, result, "started-at", "Line %d should contain started-at", i+1)
						}
					}
				} else {
					assert.Equal(t, tt.expectedType, item["type"], "Line %d should have correct type", i+1)
				}
			}
		})
	}
}

func TestExportEvidenceToConsole(t *testing.T) {
	testData := CustomEvidenceOutput{
		SchemaVersion: SchemaVersion,
		Type:          ArtifactType,
		Result: CustomEvidenceResult{
			RepoPath: "test-repo/path/file.txt",
			Evidence: []EvidenceEntry{
				{CreatedBy: "user1", PredicateSlug: "", DownloadPath: "", Verified: false, SigningKey: nil, Subject: nil, CreatedAt: ""},
			},
		},
	}

	inputJSON, err := json.Marshal(testData)
	assert.NoError(t, err)

	// Test JSON format
	err = exportEvidenceToJsonFile(inputJSON, "")
	assert.NoError(t, err)

	// Test JSONL format
	err = exportEvidenceToJsonlFile(inputJSON, "")
	assert.NoError(t, err)
}
