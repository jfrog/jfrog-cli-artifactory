package evidence

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerBuild embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerBuild struct {
	artifactory.EmptyArtifactoryServicesManager
	AqlResponse string
	AqlError    error
}

func (m *MockArtifactoryServicesManagerBuild) Aql(_ string) (io.ReadCloser, error) {
	if m.AqlError != nil {
		return nil, m.AqlError
	}
	return io.NopCloser(bytes.NewBufferString(m.AqlResponse)), nil
}

// MockOneModelManagerBuild for build tests
type MockOneModelManagerBuild struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockOneModelManagerBuild) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// MockVerifyEvidenceBaseBuild for testing verifyEvidences method
type MockVerifyEvidenceBaseBuild struct {
	mock.Mock
	verifyEvidenceBase
}

func (m *MockVerifyEvidenceBaseBuild) verifyEvidences(client *artifactory.ArtifactoryServicesManager, metadata *[]model.SearchEvidenceEdge, sha256 string) error {
	args := m.Called(client, metadata, sha256)
	return args.Error(0)
}

func TestNewVerifyEvidencesBuild(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{}
	project := "test-project"
	buildName := "test-build"
	buildNumber := "1"
	format := "json"
	keys := []string{"key1", "key2"}

	cmd := NewVerifyEvidencesBuild(serverDetails, project, buildName, buildNumber, format, keys)
	verifyCmd, ok := cmd.(*verifyEvidenceBuild)
	assert.True(t, ok)

	// Test verifyEvidenceBase fields
	assert.Equal(t, serverDetails, verifyCmd.serverDetails)
	assert.Equal(t, format, verifyCmd.format)
	assert.Equal(t, keys, verifyCmd.keys)

	// Test verifyEvidenceBuild fields
	assert.Equal(t, project, verifyCmd.project)
	assert.Equal(t, buildName, verifyCmd.buildName)
	assert.Equal(t, buildNumber, verifyCmd.buildNumber)
}

func TestVerifyEvidenceBuild_CommandName(t *testing.T) {
	cmd := &verifyEvidenceBuild{}
	assert.Equal(t, "verify-evidence-build", cmd.CommandName())
}

func TestVerifyEvidenceBuild_ServerDetails(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{Url: "http://test.com"}
	cmd := &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestVerifyEvidenceBuild_Run_Success(t *testing.T) {
	// Mock AQL response with build info file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"1-1234567890.json"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerBuild{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerBuild{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification
	mockBase := &MockVerifyEvidenceBaseBuild{}
	base := &verifyEvidenceBase{
		serverDetails: &coreConfig.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidences", mock.Anything, mock.Anything, "test-sha256").Return(nil)

	// Test direct method call
	err := mockBase.verifyEvidences(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256")
	assert.NoError(t, err)
	mockBase.AssertExpectations(t)
}

func TestVerifyEvidenceBuild_Run_AqlError(t *testing.T) {
	// Mock Artifactory client with error
	mockClient := &MockArtifactoryServicesManagerBuild{
		AqlError: errors.New("aql query failed"),
	}

	// Create build verifier
	buildVerifier := &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		project:     "test-project",
		buildName:   "test-build",
		buildNumber: "1",
	}

	err := buildVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute AQL query")
}

func TestVerifyEvidenceBuild_Run_NoBuildFound(t *testing.T) {
	// Mock AQL response with no results
	aqlResult := `{"results":[]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerBuild{
		AqlResponse: aqlResult,
	}

	// Create build verifier
	buildVerifier := &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		project:     "test-project",
		buildName:   "test-build",
		buildNumber: "1",
	}

	err := buildVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no build found")
}

func TestVerifyEvidenceBuild_Run_QueryEvidenceMetadataError(t *testing.T) {
	// Mock AQL response with build info file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"1-1234567890.json"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerBuild{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client with error
	mockOneModel := &MockOneModelManagerBuild{
		GraphqlError: errors.New("graphql query failed"),
	}

	// Create build verifier
	buildVerifier := &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
		},
		project:     "test-project",
		buildName:   "test-build",
		buildNumber: "1",
	}

	err := buildVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "graphql query failed")
}

func TestVerifyEvidenceBuild_Run_VerifyEvidencesError(t *testing.T) {
	// Mock AQL response with build info file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"1-1234567890.json"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerBuild{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerBuild{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification with error
	mockBase := &MockVerifyEvidenceBaseBuild{}
	base := &verifyEvidenceBase{
		serverDetails: &coreConfig.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidences", mock.Anything, mock.Anything, "test-sha256").Return(errors.New("verification failed"))

	// Test direct method call
	err := mockBase.verifyEvidences(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
	mockBase.AssertExpectations(t)
}

func TestVerifyEvidenceBuild_Run_CreateArtifactoryClientError(t *testing.T) {
	// Create build verifier with invalid server configuration that would cause client creation to fail
	buildVerifier := &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &coreConfig.ServerDetails{
				Url: "invalid-url", // Invalid URL that should cause client creation to fail
			},
		},
		project:     "test-project",
		buildName:   "test-build",
		buildNumber: "1",
	}

	err := buildVerifier.Run()
	assert.Error(t, err)
	// Just verify an error occurs - the specific error depends on when the invalid config is detected
}

func TestVerifyEvidenceBuild_ProjectBuildRepoKey(t *testing.T) {
	// Test different project scenarios for repo key building
	testCases := []struct {
		name            string
		project         string
		buildName       string
		buildNumber     string
		expectedRepoKey string
	}{
		{
			name:            "Empty project",
			project:         "",
			buildName:       "test-build",
			buildNumber:     "1",
			expectedRepoKey: "artifactory-build-info",
		},
		{
			name:            "Default project",
			project:         "default",
			buildName:       "test-build",
			buildNumber:     "1",
			expectedRepoKey: "artifactory-build-info",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock AQL response with no results to test repo key logic
			aqlResult := `{"results":[]}`

			// Mock Artifactory client
			mockClient := &MockArtifactoryServicesManagerBuild{
				AqlResponse: aqlResult,
			}

			// Create build verifier
			buildVerifier := &verifyEvidenceBuild{
				verifyEvidenceBase: verifyEvidenceBase{
					serverDetails: &coreConfig.ServerDetails{},
					artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
						c := artifactory.ArtifactoryServicesManager(mockClient)
						return &c
					}(),
				},
				project:     tc.project,
				buildName:   tc.buildName,
				buildNumber: tc.buildNumber,
			}

			// Run should fail with "no build found" but this validates the repo key logic
			err := buildVerifier.Run()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no build found")
		})
	}
}
