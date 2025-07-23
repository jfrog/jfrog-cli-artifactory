package verify

import (
	"bytes"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerVerifier embeds EmptyArtifactoryServicesManager and overrides methods for testing
type MockArtifactoryServicesManagerVerifier struct {
	artifactory.EmptyArtifactoryServicesManager
	ReadRemoteFileResponse io.ReadCloser
	ReadRemoteFileError    error
}

func (m *MockArtifactoryServicesManagerVerifier) ReadRemoteFile(_ string) (io.ReadCloser, error) {
	if m.ReadRemoteFileError != nil {
		return nil, m.ReadRemoteFileError
	}
	return m.ReadRemoteFileResponse, nil
}

// MockArtifactoryServicesManagerVerifierStrict - fails if key-fetching methods are called
type MockArtifactoryServicesManagerVerifierStrict struct {
	artifactory.EmptyArtifactoryServicesManager
	ReadRemoteFileResponse io.ReadCloser
	ReadRemoteFileError    error
	t                      *testing.T
}

func (m *MockArtifactoryServicesManagerVerifierStrict) ReadRemoteFile(_ string) (io.ReadCloser, error) {
	if m.ReadRemoteFileError != nil {
		return nil, m.ReadRemoteFileError
	}
	return m.ReadRemoteFileResponse, nil
}

// MockDSSEVerifier for testing DSSE verification
type MockDSSEVerifier struct {
	mock.Mock
	VerifyError error
	KeyIDValue  string
	PublicKey   crypto.PublicKey
}

func (m *MockDSSEVerifier) Verify(data, signature []byte) error {
	args := m.Called(data, signature)
	if m.VerifyError != nil {
		return m.VerifyError
	}
	return args.Error(0)
}

func (m *MockDSSEVerifier) KeyID() (string, error) {
	return m.KeyIDValue, nil
}

func (m *MockDSSEVerifier) Public() crypto.PublicKey {
	return m.PublicKey
}

// Helper functions to create mock data

func createMockEnvelope() dsse.Envelope {
	return dsse.Envelope{
		Payload:     "eyJ0ZXN0IjoiZGF0YSJ9",
		PayloadType: "application/vnd.in-toto+json",
		Signatures: []dsse.Signature{
			{
				KeyId: "test-key-id",
				Sig:   "dGVzdC1zaWduYXR1cmU=",
			},
		},
	}
}

func createMockEnvelopeBytes() []byte {
	envelope := createMockEnvelope()
	data, err := json.Marshal(envelope)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal mock envelope: %v", err))
	}
	return data
}

// Test Verify with nil evidence metadata
func TestVerifier_Verify_NilEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, true, &clientInterface)

	result, err := verifier.Verify("test-sha256", nil, "")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no evidence metadata provided")
}

// Test Verify with empty evidence metadata
func TestVerifier_Verify_EmptyEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, true, &clientInterface)
	emptyMetadata := &[]model.SearchEvidenceEdge{}

	result, err := verifier.Verify("test-sha256", emptyMetadata, "")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no evidence metadata provided")
}

// Test Verify with invalid evidence metadata (nil node)
func TestVerifier_Verify_InvalidEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, false, &clientInterface)

	// Create evidence metadata with empty SHA256 - this should still process but fail checksum verification
	invalidMetadata := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "/test/path", // Provide a download path to avoid nil pointer
				PredicateType: "test-predicate",
				Subject:       model.EvidenceSubject{Sha256: ""}, // Empty SHA256
			},
		},
	}

	result, err := verifier.Verify("test-sha256", invalidMetadata, "")

	// Should not error during processing, but checksum verification should fail
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.OverallVerificationStatus)
	assert.Len(t, *result.EvidenceVerifications, 1)
	// The checksum verification should fail due to mismatch
	assert.Equal(t, model.VerificationStatus(model.Failed), (*result.EvidenceVerifications)[0].VerificationResult.Sha256VerificationStatus)
}

// Test readEnvelopeFromRemote function directly (tests envelope reading without triggering remote key fetching)
func TestReadEnvelopeFromRemote_Success(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
	}

	verifier := evidenceVerifier{
		artifactoryClient: mockClient,
	}

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}

	envelope, err := verifier.readEnvelope(edge)

	assert.NoError(t, err)
	assert.Equal(t, "eyJ0ZXN0IjoiZGF0YSJ9", envelope.Payload)
	assert.Equal(t, "application/vnd.in-toto+json", envelope.PayloadType)
	assert.Equal(t, 1, len(envelope.Signatures))
	assert.Equal(t, "test-key-id", envelope.Signatures[0].KeyId)
	assert.Equal(t, "dGVzdC1zaWduYXR1cmU=", envelope.Signatures[0].Sig)
}

// Test readEnvelopeFromRemote with read error
func TestReadEnvelopeFromRemote_ReadError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileError: errors.New("failed to read remote file"),
	}

	verifier := evidenceVerifier{
		artifactoryClient: mockClient,
	}

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}

	envelope, err := verifier.readEnvelope(edge)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read remote file")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test readEnvelopeFromRemote with invalid JSON
func TestReadEnvelopeFromRemote_InvalidJSON(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader([]byte("invalid json"))),
	}

	verifier := evidenceVerifier{
		artifactoryClient: mockClient,
	}

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}

	envelope, err := verifier.readEnvelope(edge)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal envelope")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test readEnvelopeFromRemote with empty file content
func TestReadEnvelopeFromRemote_EmptyFile(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader([]byte{})),
	}

	verifier := evidenceVerifier{
		artifactoryClient: mockClient,
	}

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}

	envelope, err := verifier.readEnvelope(edge)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal envelope")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test verifyEnvelope with successful verification
func TestVerifyEnvelop_SuccessfulVerification(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key",
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := createMockEnvelope()
	result := &model.EvidenceVerification{}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.True(t, success)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test verifyEnvelope with failed verification
func TestVerifyEnvelop_FailedVerification(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue:  "test-key",
		VerifyError: errors.New("verification failed"),
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockEnvelope()
	result := &model.EvidenceVerification{}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test verifyEnvelope with nil inputs
func TestVerifyEnvelop_NilInputs(t *testing.T) {
	result := &model.EvidenceVerification{}

	success := verifyEnvelope(nil, nil, result)

	assert.False(t, success)
	// When inputs are nil, verifyEnvelope doesn't set the status (returns early)
	assert.Equal(t, model.VerificationStatus("failed"), result.VerificationResult.SignaturesVerificationStatus)
}

// Test verifyEnvelope with empty verifiers
func TestVerifyEnvelop_EmptyVerifiers(t *testing.T) {
	envelope := createMockEnvelope()
	result := &model.EvidenceVerification{}

	success := verifyEnvelope([]dsse.Verifier{}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

// Test verifyEnvelope with empty signatures
func TestVerifyEnvelop_EmptySignatures(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key",
	}

	envelope := dsse.Envelope{
		Payload:     "eyJ0ZXN0IjoiZGF0YSJ9",
		PayloadType: "application/vnd.in-toto+json",
		Signatures:  []dsse.Signature{}, // Empty signatures
	}
	result := &model.EvidenceVerification{}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

// Test verifyEnvelope with mismatched key IDs
func TestVerifyEnvelop_MismatchedKeyIDs(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "different-key", // Different from signature key ID
	}
	// Set up the mock to return an error (verification failure)
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockEnvelope() // Contains "test-key-id"
	result := &model.EvidenceVerification{}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test Verify with successful verification using local keys
func TestVerifier_Verify_Success(t *testing.T) {
	// Create mock DSSE verifier that will succeed
	mockDSSEVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
		PublicKey:  nil, // Can be nil for this test
	}
	// Set up the mock to return success (no error)
	mockDSSEVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	// Create mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	// Create verifier with pre-set local keys to bypass file reading
	verifier := &evidenceVerifier{
		keys:               []string{"test-key.pem"}, // Not used since we set localKeys directly
		useArtifactoryKeys: false,                    // Only use local keys
		artifactoryClient:  clientInterface,
		localKeys:          []dsse.Verifier{mockDSSEVerifier}, // Bypass file reading
	}

	// Create mock evidence metadata
	evidenceMetadata := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				Subject: model.EvidenceSubject{
					Sha256: "test-sha256",
				},
				DownloadPath:  "/evidence/path",
				PredicateType: "test-predicate",
				CreatedBy:     "test-user",
				CreatedAt:     "2023-01-01T00:00:00Z",
			},
		},
	}

	// Call Verify method
	result, err := verifier.Verify("test-sha256", evidenceMetadata, "/test/subject/path")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "/test/subject/path", result.Subject.Path)
	assert.Equal(t, "test-sha256", result.Subject.Sha256)
	assert.Equal(t, model.VerificationStatus(model.Success), result.OverallVerificationStatus)

	// Check evidence verifications
	assert.NotNil(t, result.EvidenceVerifications)
	assert.Len(t, *result.EvidenceVerifications, 1)

	evidence := (*result.EvidenceVerifications)[0]
	assert.Equal(t, "/evidence/path", evidence.DownloadPath)
	assert.Equal(t, "test-sha256", evidence.SubjectChecksum)
	assert.Equal(t, "test-predicate", evidence.PredicateType)
	assert.Equal(t, "test-user", evidence.CreatedBy)
	assert.Equal(t, "2023-01-01T00:00:00Z", evidence.CreatedAt)
	assert.Equal(t, model.VerificationStatus(model.Success), evidence.VerificationResult.Sha256VerificationStatus)
	assert.Equal(t, model.VerificationStatus(model.Success), evidence.VerificationResult.SignaturesVerificationStatus)
	assert.Equal(t, localKeySource, evidence.VerificationResult.KeySource)

	// Verify mock was called
	mockDSSEVerifier.AssertExpectations(t)
}

// Test Verify with verification failure using local keys
func TestVerifier_Verify_VerificationFailed(t *testing.T) {
	// Create mock DSSE verifier that will fail verification
	mockDSSEVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
		PublicKey:  nil,
	}
	// Set up the mock to return failure (error)
	mockDSSEVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("signature verification failed"))

	// Create mock Artifactory client
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	// Create verifier with pre-set local keys to bypass file reading
	verifier := &evidenceVerifier{
		keys:               []string{"test-key.pem"},
		useArtifactoryKeys: false, // Only use local keys
		artifactoryClient:  clientInterface,
		localKeys:          []dsse.Verifier{mockDSSEVerifier}, // Bypass file reading
	}

	// Create mock evidence metadata
	evidenceMetadata := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				Subject: model.EvidenceSubject{
					Sha256: "test-sha256",
				},
				DownloadPath:  "/evidence/path",
				PredicateType: "test-predicate",
				CreatedBy:     "test-user",
				CreatedAt:     "2023-01-01T00:00:00Z",
			},
		},
	}

	// Call Verify method
	result, err := verifier.Verify("test-sha256", evidenceMetadata, "/test/subject/path")

	// Assertions - should succeed but with failed verification status
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "/test/subject/path", result.Subject.Path)
	assert.Equal(t, "test-sha256", result.Subject.Sha256)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.OverallVerificationStatus)

	// Check evidence verifications
	assert.NotNil(t, result.EvidenceVerifications)
	assert.Len(t, *result.EvidenceVerifications, 1)

	evidence := (*result.EvidenceVerifications)[0]
	assert.Equal(t, "/evidence/path", evidence.DownloadPath)
	assert.Equal(t, "test-sha256", evidence.SubjectChecksum)
	assert.Equal(t, model.VerificationStatus(model.Success), evidence.VerificationResult.Sha256VerificationStatus)
	assert.Equal(t, model.VerificationStatus(model.Failed), evidence.VerificationResult.SignaturesVerificationStatus)

	// Verify mock was called
	mockDSSEVerifier.AssertExpectations(t)
}

func TestVerifier_Verify_UseArtifactoryKeysFalse_NoArtifactoryClientCalls(t *testing.T) {
	// Create mock DSSE verifier that will succeed
	mockDSSEVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
		PublicKey:  nil,
	}
	// Set up the mock to return success (no error)
	mockDSSEVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	// Create strict mock that will fail the test if GetTrustedKeys or GetKeyPair are called
	mockClient := &MockArtifactoryServicesManagerVerifierStrict{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
		t:                      t,
	}

	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	// Create verifier with useArtifactoryKeys=false and pre-set local keys
	verifier := &evidenceVerifier{
		keys:               []string{"test-key.pem"},
		useArtifactoryKeys: false, // This should prevent any Artifactory client calls for key fetching
		artifactoryClient:  clientInterface,
		localKeys:          []dsse.Verifier{mockDSSEVerifier}, // Bypass file reading
	}

	// Create mock evidence metadata
	evidenceMetadata := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				Subject: model.EvidenceSubject{
					Sha256: "test-sha256",
				},
				DownloadPath:  "/evidence/path",
				PredicateType: "test-predicate",
				CreatedBy:     "test-user",
				CreatedAt:     "2023-01-01T00:00:00Z",
			},
		},
	}

	// Call Verify method - if GetTrustedKeys/GetKeyPair are called, the test will fail
	result, err := verifier.Verify("test-sha256", evidenceMetadata, "/test/subject/path")

	// Assertions - should succeed with local key verification
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, model.VerificationStatus(model.Success), result.OverallVerificationStatus)

	evidence := (*result.EvidenceVerifications)[0]
	assert.Equal(t, localKeySource, evidence.VerificationResult.KeySource)
	assert.Equal(t, model.VerificationStatus(model.Success), evidence.VerificationResult.SignaturesVerificationStatus)

	// Verify that the DSSE verifier was called
	mockDSSEVerifier.AssertExpectations(t)
}
