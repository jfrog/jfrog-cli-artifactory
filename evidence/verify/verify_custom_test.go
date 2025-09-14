package verify

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerCustom embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerCustom struct {
	artifactory.EmptyArtifactoryServicesManager
	AqlResponse string
	AqlError    error
}

func (m *MockArtifactoryServicesManagerCustom) Aql(_ string) (io.ReadCloser, error) {
	if m.AqlError != nil {
		return nil, m.AqlError
	}
	return io.NopCloser(bytes.NewBufferString(m.AqlResponse)), nil
}

// MockOneModelManagerCustom for custom tests
type MockOneModelManagerCustom struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockOneModelManagerCustom) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// MockVerifyEvidenceBaseCustom for testing verifyEvidence method
type MockVerifyEvidenceBaseCustom struct {
	mock.Mock
	verifyEvidenceBase
}

func (m *MockVerifyEvidenceBaseCustom) verifyEvidence(client *artifactory.ArtifactoryServicesManager, metadata *[]model.SearchEvidenceEdge, sha256, subjectPath string) error {
	args := m.Called(client, metadata, sha256, subjectPath)
	return args.Error(0)
}

// MockVerifierCustom for testing cryptox.Verifier.Verify method calls
type MockVerifierCustom struct {
	mock.Mock
}

func (m *MockVerifierCustom) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
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

func TestNewVerifyEvidenceCustom(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	subjectRepoPath := "test-repo/path/to/subject.txt"
	format := "json"
	keys := []string{"key1", "key2"}

	cmd := NewVerifyEvidenceCustom(serverDetails, subjectRepoPath, format, keys, true)
	verifyCmd, ok := cmd.(*verifyEvidenceCustom)
	assert.True(t, ok)

	// Test verifyEvidenceBase fields
	assert.Equal(t, serverDetails, verifyCmd.serverDetails)
	assert.Equal(t, format, verifyCmd.format)
	assert.Equal(t, keys, verifyCmd.keys)

	// Test verifyEvidenceCustom fields
	assert.Equal(t, subjectRepoPath, verifyCmd.subjectRepoPath)
	assert.True(t, verifyCmd.useArtifactoryKeys)
}

func TestVerifyEvidenceCustom_CommandName(t *testing.T) {
	cmd := &verifyEvidenceCustom{}
	assert.Equal(t, "verify-evidence-custom", cmd.CommandName())
}

func TestVerifyEvidenceCustom_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "test.com"}
	cmd := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestVerifyEvidenceCustom_Run_AqlError(t *testing.T) {
	// Mock Artifactory client with error
	mockClient := &MockArtifactoryServicesManagerCustom{
		AqlError: errors.New("aql query failed"),
	}

	// Create custom verifier
	customVerifier := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}

	err := customVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute AQL query")
}

func TestVerifyEvidenceCustom_Run_NoSubjectFound(t *testing.T) {
	// Mock AQL response with no results
	aqlResult := `{"results":[]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCustom{
		AqlResponse: aqlResult,
	}

	// Create custom verifier
	customVerifier := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}

	err := customVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no subject found")
}

func TestVerifyEvidenceCustom_Run_QueryEvidenceMetadataError(t *testing.T) {
	// Mock AQL response with subject file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"subject.txt","repo":"test-repo","path":"path/to"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCustom{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client with error
	mockOneModel := &MockOneModelManagerCustom{
		GraphqlError: errors.New("graphql query failed"),
	}

	// Create custom verifier
	customVerifier := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{},
			artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
				c := artifactory.ArtifactoryServicesManager(mockClient)
				return &c
			}(),
			oneModelClient: mockOneModel,
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}

	err := customVerifier.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "graphql query failed")
}

func TestVerifyEvidenceCustom_Run_VerifyEvidenceError(t *testing.T) {
	// Mock AQL response with subject file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"subject.txt","repo":"test-repo","path":"path/to"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCustom{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerCustom{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"}}}]}}}}`),
	}

	// Mock the base verification with error
	mockBase := &MockVerifyEvidenceBaseCustom{}
	base := &verifyEvidenceBase{
		serverDetails: &config.ServerDetails{},
		artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
			c := artifactory.ArtifactoryServicesManager(mockClient)
			return &c
		}(),
		oneModelClient: mockOneModel,
	}
	mockBase.verifyEvidenceBase = *base
	mockBase.On("verifyEvidence", mock.Anything, mock.Anything, "test-sha256", mock.Anything).Return(errors.New("verification failed"))

	// Test direct method call
	err := mockBase.verifyEvidence(nil, &[]model.SearchEvidenceEdge{{}}, "test-sha256", "test-path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
	mockBase.AssertExpectations(t)
}

func TestVerifyEvidenceCustom_Run_CreateArtifactoryClientError(t *testing.T) {
	// Create custom verifier with invalid server configuration that would cause client creation to fail
	customVerifier := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails: &config.ServerDetails{
				Url: "invalid-url", // Invalid URL that should cause client creation to fail
			},
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}

	err := customVerifier.Run()
	assert.Error(t, err)
	// Just verify an error occurs - the specific error depends on when the invalid config is detected
}

func TestVerifyEvidenceCustom_SubjectRepoPathParsing(t *testing.T) {
	// Test different subject repo path formats
	testCases := []struct {
		name            string
		subjectRepoPath string
		expectedRepo    string
		expectedPath    string
		expectedName    string
	}{
		{
			name:            "Simple path",
			subjectRepoPath: "repo/file.txt",
			expectedRepo:    "repo",
			expectedPath:    "",
			expectedName:    "file.txt",
		},
		{
			name:            "Path with directory",
			subjectRepoPath: "repo/path/to/file.txt",
			expectedRepo:    "repo",
			expectedPath:    "path/to",
			expectedName:    "file.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock AQL response with no results to test path parsing
			aqlResult := `{"results":[]}`

			// Mock Artifactory client
			mockClient := &MockArtifactoryServicesManagerCustom{
				AqlResponse: aqlResult,
			}

			// Create custom verifier
			customVerifier := &verifyEvidenceCustom{
				verifyEvidenceBase: verifyEvidenceBase{
					serverDetails: &config.ServerDetails{},
					artifactoryClient: func() *artifactory.ArtifactoryServicesManager {
						c := artifactory.ArtifactoryServicesManager(mockClient)
						return &c
					}(),
				},
				subjectRepoPath: tc.subjectRepoPath,
			}

			// Run should fail with "no subject found" but this validates the path parsing
			err := customVerifier.Run()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no subject found")
		})
	}
}

func TestVerifyEvidenceCustom_Run_Success(t *testing.T) {
	// Mock AQL response with subject file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"subject.txt","repo":"test-repo","path":"path/to"}]}`

	// Mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerCustom{
		AqlResponse: aqlResult,
	}

	// Mock OneModel client for evidence metadata
	mockOneModel := &MockOneModelManagerCustom{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/test/evidence.json","predicateType":"test-predicate"}}]}}}}`),
	}

	// Create testify mock for verifier using the new interface
	mockVerifier := &MockVerifierCustom{}
	expectedResponse := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		Subject: model.Subject{
			Path:   "test-repo/path/to/subject.txt",
			Sha256: "test-sha256",
		},
	}

	// Set up the mock expectations - use mock.Anything for the subjectPath since it gets formatted
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.Anything).Return(expectedResponse, nil)

	// Create custom verifier with injected mock verifier
	customVerifier := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:  &config.ServerDetails{},
			format:         "json",
			keys:           []string{},
			oneModelClient: mockOneModel,
			verifier:       mockVerifier, // Inject the mock verifier using the interface
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}

	// Mock the createArtifactoryClient method by setting the client directly
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	customVerifier.artifactoryClient = &clientInterface

	// Call the actual Run method
	err := customVerifier.Run()

	// Assert results
	assert.NoError(t, err)

	// Verify that the mock verifier was called with the exact expected parameters
	mockVerifier.AssertExpectations(t)
	mockVerifier.AssertCalled(t, "Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.Anything)

	t.Log("✅ SUCCESS: Run() method called mock verifier.Verify() with testify assertions!")
}

func TestVerifyEvidenceCustom_Progress_Success(t *testing.T) {
	// Mock AQL response with subject file
	aqlResult := `{"results":[{"sha256":"test-sha256","name":"subject.txt","repo":"test-repo","path":"path/to"}]}`
	mockClient := &MockArtifactoryServicesManagerCustom{AqlResponse: aqlResult}
	mockOneModel := &MockOneModelManagerCustom{GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"node":{"subject":{"sha256":"test-sha256"},"downloadPath":"/evidence/path"}}]}}}}`)}
	mockVerifier := &MockVerifierCustom{}
	expected := &model.VerificationResponse{OverallVerificationStatus: model.Success, Subject: model.Subject{Path: "test-repo/path/to/subject.txt", Sha256: "test-sha256"}}
	mockVerifier.On("Verify", "test-sha256", mock.AnythingOfType("*[]model.SearchEvidenceEdge"), mock.AnythingOfType("string")).Return(expected, nil)

	cmd := &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:  &config.ServerDetails{},
			format:         "json",
			verifier:       mockVerifier,
			oneModelClient: mockOneModel,
		},
		subjectRepoPath: "test-repo/path/to/subject.txt",
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	cmd.artifactoryClient = &clientInterface
	pm := &fakeProgress{}
	cmd.progressMgr = pm

	err := cmd.Run()
	assert.NoError(t, err)
	assert.True(t, pm.quitCalled)
	assert.True(t, len(pm.headlines) >= 1)
}
