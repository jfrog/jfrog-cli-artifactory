package get

import (
	"encoding/json"
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
	cmd := NewGetEvidenceReleaseBundle(serverDetails, "myBundle", SchemaVersion, "myProject", "json", "output.json", "1000", true)

	bundle, ok := cmd.(*getEvidenceReleaseBundle)

	assert.True(t, ok)
	assert.IsType(t, &getEvidenceReleaseBundle{}, bundle)
	assert.Equal(t, serverDetails, bundle.serverDetails)
	assert.Equal(t, "myBundle", bundle.releaseBundle)
	assert.Equal(t, SchemaVersion, bundle.releaseBundleVersion)
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
				releaseBundleVersion: SchemaVersion,
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
			releaseBundleVersion: SchemaVersion,
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

func TestTransformReleaseBundleGraphQLOutput(t *testing.T) {
	g := &getEvidenceReleaseBundle{
		releaseBundle:        "test-bundle",
		releaseBundleVersion: SchemaVersion,
		getEvidenceBase: getEvidenceBase{
			includePredicate: false,
		},
	}

	inputStr, err := ReadTestDataFile("release_bundle_complex_input.json")
	assert.NoError(t, err)

	result, err := g.transformReleaseBundleGraphQLOutput([]byte(inputStr))
	assert.NoError(t, err)

	// Parse the result to verify structure
	var output ReleaseBundleOutput
	err = json.Unmarshal(result, &output)
	assert.NoError(t, err)

	// Check top-level fields
	assert.Equal(t, SchemaVersion, output.SchemaVersion)
	assert.Equal(t, ReleaseBundleType, output.Type)

	// Check result structure
	assert.Equal(t, "test-bundle", output.Result.ReleaseBundle)
	assert.Equal(t, SchemaVersion, output.Result.ReleaseBundleVersion)

	// Check release bundle evidence
	assert.Len(t, output.Result.Evidence, 1)

	// Check first evidence entry
	firstEvidence := output.Result.Evidence[0]
	assert.Equal(t, "jfxr@01j1ww94gjdccy7x8f8g2vdp25", firstEvidence.CreatedBy)
	assert.Equal(t, "cyclonedx-sbom", firstEvidence.PredicateSlug)
	assert.Equal(t, true, firstEvidence.Verified)

	// Check artifacts
	assert.Len(t, output.Result.Artifacts, 1)

	firstArtifact := output.Result.Artifacts[0]
	assert.Equal(t, "greenpizza-docker-dev/call-moderation/48/list.manifest.json", firstArtifact.RepoPath)
	assert.Equal(t, "docker", firstArtifact.PackageType)

	// Check builds
	assert.Len(t, output.Result.Builds, 1)

	firstBuild := output.Result.Builds[0]
	assert.Equal(t, "greenpizza-build", firstBuild.BuildName)
	assert.Equal(t, "48", firstBuild.BuildNumber)
	assert.Equal(t, "2024-12-02T07:17:48.109Z", firstBuild.StartedAt)
}

func TestTransformReleaseBundleGraphQLOutputWithPredicate(t *testing.T) {
	g := &getEvidenceReleaseBundle{
		releaseBundle:        "test-bundle",
		releaseBundleVersion: SchemaVersion,
		getEvidenceBase: getEvidenceBase{
			includePredicate: true,
		},
	}

	inputStr, err := ReadTestDataFile("release_bundle_predicate_input.json")
	assert.NoError(t, err)

	result, err := g.transformReleaseBundleGraphQLOutput([]byte(inputStr))
	assert.NoError(t, err)

	var output ReleaseBundleOutput
	err = json.Unmarshal(result, &output)
	assert.NoError(t, err)

	assert.Len(t, output.Result.Evidence, 1)

	firstEvidence := output.Result.Evidence[0]
	assert.Equal(t, map[string]any{"analysis": "sbom"}, firstEvidence.Predicate)
}

func TestTransformReleaseBundleGraphQLOutputEmptyResponse(t *testing.T) {
	g := &getEvidenceReleaseBundle{
		releaseBundle:        "test-bundle",
		releaseBundleVersion: SchemaVersion,
		getEvidenceBase: getEvidenceBase{
			includePredicate: false,
		},
	}

	inputStr, err := ReadTestDataFile("release_bundle_empty_input.json")
	assert.NoError(t, err)

	result, err := g.transformReleaseBundleGraphQLOutput([]byte(inputStr))
	assert.NoError(t, err)

	var output ReleaseBundleOutput
	err = json.Unmarshal(result, &output)
	assert.NoError(t, err)

	assert.Len(t, output.Result.Evidence, 0)

	// Should not have artifacts or builds fields when empty
	assert.Len(t, output.Result.Artifacts, 0)
	assert.Len(t, output.Result.Builds, 0)
}

func TestTransformReleaseBundleGraphQLOutputInvalidStructure(t *testing.T) {
	g := &getEvidenceReleaseBundle{
		releaseBundle:        "test-bundle",
		releaseBundleVersion: SchemaVersion,
		getEvidenceBase: getEvidenceBase{
			includePredicate: false,
		},
	}

	// Test with invalid JSON
	_, err := g.transformReleaseBundleGraphQLOutput([]byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse GraphQL response")

	// Test with missing data field
	input := `{"someOtherField": "value"}`
	_, err = g.transformReleaseBundleGraphQLOutput([]byte(input))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing data field")

	// Test with missing releaseBundleVersion field
	input = `{"data": {"someOtherField": "value"}}`
	_, err = g.transformReleaseBundleGraphQLOutput([]byte(input))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing releaseBundleVersion field")
}
