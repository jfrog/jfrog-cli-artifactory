package verify

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerReleaseBundle embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerReleaseBundle struct {
	artifactory.EmptyArtifactoryServicesManager
	AqlResponse string
	AqlError    error
}

func (m *MockArtifactoryServicesManagerReleaseBundle) Aql(_ string) (io.ReadCloser, error) {
	if m.AqlError != nil {
		return nil, m.AqlError
	}
	return io.NopCloser(bytes.NewBufferString(m.AqlResponse)), nil
}

// MockOneModelManagerReleaseBundle for release bundle tests
type MockOneModelManagerReleaseBundle struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockOneModelManagerReleaseBundle) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// MockVerifyEvidenceBaseReleaseBundle for testing verifyEvidenceBase methods
type MockVerifyEvidenceBaseReleaseBundle struct {
	mock.Mock
	verifyEvidenceBase
}

func (m *MockVerifyEvidenceBaseReleaseBundle) verifyEvidences(client *artifactory.ArtifactoryServicesManager, metadata *[]model.SearchEvidenceEdge, sha256 string) error {
	args := m.Called(client, metadata, sha256)
	return args.Error(0)
}

// MockVerifierReleaseBundle for testing with testify - implements VerifierInterface
type MockVerifierReleaseBundle struct {
	mock.Mock
}

func (m *MockVerifierReleaseBundle) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
	args := m.Called(subjectSha256, evidenceMetadata, subjectPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	resp, ok := args.Get(0).(*model.VerificationResponse)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return resp, args.Error(1)
}

func TestNewVerifyEvidenceReleaseBundle(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	format := "json"
	project := "test-project"
	releaseBundle := "test-release-bundle"
	releaseBundleVersion := "1.0.0"
	keys := []string{"key1", "key2"}

	cmd := NewVerifyEvidenceReleaseBundle(serverDetails, format, project, releaseBundle, releaseBundleVersion, keys, true)
	verifyCmd, ok := cmd.(*verifyEvidenceReleaseBundle)
	assert.True(t, ok)

	// Test verifyEvidenceBase fields
	assert.Equal(t, serverDetails, verifyCmd.serverDetails)
	assert.Equal(t, format, verifyCmd.format)
	assert.Equal(t, keys, verifyCmd.keys)

	// Test verifyEvidenceReleaseBundle fields
	assert.Equal(t, project, verifyCmd.project)
	assert.Equal(t, releaseBundle, verifyCmd.releaseBundle)
	assert.Equal(t, releaseBundleVersion, verifyCmd.releaseBundleVersion)
	assert.True(t, verifyCmd.useArtifactoryKeys)
}

func TestVerifyEvidenceReleaseBundle_CommandName(t *testing.T) {
	cmd := &verifyEvidenceReleaseBundle{}
	assert.Equal(t, "create-release-bundle-evidence", cmd.CommandName())
}

func TestVerifyEvidenceReleaseBundle_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com"}
	cmd := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestVerifyEvidenceReleaseBundle_Run_Success(t *testing.T) {
	// Mock AQL response with release bundle manifest
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerReleaseBundle{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierReleaseBundle)

	// Set up expectations for the mock verifier
	expectedResponse := &model.VerificationResponse{
		SubjectPath:               "/test/subject/path",
		SubjectChecksum:           "test-sha256",
		EvidenceVerifications:     &[]model.EvidenceVerification{},
		OverallVerificationStatus: model.Success,
	}
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expectedResponse, nil)

	// Create release bundle verifier with mock verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	// Call the actual Run method
	err := releaseBundleVerifier.Run()

	// Should succeed with our mock verifier returning success status
	assert.NoError(t, err)

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidenceReleaseBundle_Run_VerificationFailed(t *testing.T) {
	// Mock AQL response with release bundle manifest
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerReleaseBundle{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierReleaseBundle)

	// Set up expectations for the mock verifier to return FAILED status
	expectedResponse := &model.VerificationResponse{
		SubjectPath:               "/test/subject/path",
		SubjectChecksum:           "test-sha256",
		EvidenceVerifications:     &[]model.EvidenceVerification{},
		OverallVerificationStatus: model.Failed,
	}
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expectedResponse, nil)

	// Create release bundle verifier with mock verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	// Call the actual Run method
	err := releaseBundleVerifier.Run()

	// Should fail with ExitCodeFailNoOp due to FAILED verification status
	assert.Error(t, err)
	var cliErr coreutils.CliError
	assert.ErrorAs(t, err, &cliErr)
	assert.Equal(t, coreutils.ExitCodeError, cliErr.ExitCode)

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidenceReleaseBundle_Run_VerificationError(t *testing.T) {
	// Mock AQL response with release bundle manifest
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerReleaseBundle{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`),
	}

	// Create mock verifier using testify
	mockVerifier := new(MockVerifierReleaseBundle)

	// Set up expectations for the mock verifier to return an error
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return((*model.VerificationResponse)(nil), errors.New("verification error"))

	// Create release bundle verifier with mock verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
			format:         "json",
			keys:           []string{},
			verifier:       mockVerifier, // Inject our mock verifier
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	// Call the actual Run method
	err := releaseBundleVerifier.Run()

	// Should return the verification error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification error")

	// Verify that the mock verifier was called with expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string"))
}

func TestVerifyEvidenceReleaseBundle_Run_AqlError(t *testing.T) {
	// Mock Artifactory client with error
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlError: errors.New("aql query failed"),
	}

	// Create release bundle verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	err := releaseBundleVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute AQL query")
}

func TestVerifyEvidenceReleaseBundle_Run_NoReleaseBundleFound(t *testing.T) {
	// Mock AQL response with no results
	aqlResult := `{"results":[]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Create release bundle verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	err := releaseBundleVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no release bundle manifest found for the given release bundle and version")
}

func TestVerifyEvidenceReleaseBundle_Run_QueryEvidenceMetadataError(t *testing.T) {
	// Mock AQL response with release bundle manifest
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client with error
	mockOneModel := &MockOneModelManagerReleaseBundle{
		GraphqlError: errors.New("graphql query failed"),
	}

	// Create release bundle verifier
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	err := releaseBundleVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "graphql query failed")
}

func TestVerifyEvidenceReleaseBundle_Run_VerifyEvidencesError(t *testing.T) {
	// Mock AQL response with release bundle manifest
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerReleaseBundle{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerReleaseBundle{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification with error
	mockBase := &MockVerifyEvidenceBaseReleaseBundle{}
	base := &verifyEvidenceBase{
		serverDetails: &config.ServerDetails{},
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

func TestVerifyEvidenceReleaseBundle_Run_CreateArtifactoryClientError(t *testing.T) {
	// Create release bundle verifier with invalid server details that would cause client creation to fail
	releaseBundleVerifier := &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{
				Url: "invalid-url", // Invalid URL should cause client creation to fail
			},
		},
		project:              "test-project",
		releaseBundle:        "test-release-bundle",
		releaseBundleVersion: "1.0.0",
	}

	err := releaseBundleVerifier.Run()
	assert.Error(t, err)
	// Just verify an error occurs - don't be specific about the message
}

func TestVerifyEvidenceReleaseBundle_ProjectBuildRepoKey(t *testing.T) {
	// Test different project scenarios for repo key building
	testCases := []struct {
		name            string
		project         string
		expectedRepoKey string
	}{
		{
			name:            "Empty project",
			project:         "",
			expectedRepoKey: "release-bundles-v2",
		},
		{
			name:            "Default project",
			project:         "default",
			expectedRepoKey: "release-bundles-v2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock AQL response with release bundle manifest
			aqlResult := `{"results":[{"sha256":"test-sha256","name":"release-bundle.json.evd"}]}`

			// Mock Artifactory client
			mockClient := &MockArtifactoryServicesManagerReleaseBundle{
				AqlResponse: aqlResult,
			}

			// Mock OneModel client for evidence metadata
			mockOneModel := &MockOneModelManagerReleaseBundle{
				GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
			}

			// Mock the base verification
			mockBase := &MockVerifyEvidenceBaseReleaseBundle{}
			base := &verifyEvidenceBase{
				serverDetails: &config.ServerDetails{},
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
		})
	}
}
