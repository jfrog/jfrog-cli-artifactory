package cryptox

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
	data, _ := json.Marshal(envelope)
	return data
}

// Test NewVerifier constructor
func TestNewVerifier(t *testing.T) {
	keys := []string{"key1", "key2"}
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	// Need to convert our mock to the interface and take its address
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	verifier := NewVerifier(keys, &clientInterface)

	assert.NotNil(t, verifier)
	assert.Equal(t, keys, verifier.keys)
	assert.Equal(t, clientInterface, verifier.artifactoryClient)
}

// Test NewVerifier with nil client and details (tests graceful handling)
func TestNewVerifier_NilClientAndDetails(t *testing.T) {
	keys := []string{"key1"}

	// The NewVerifier function dereferences client and details pointers, so passing nil will panic
	// This test documents this behavior
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior - NewVerifier panics with nil pointers
			assert.Contains(t, fmt.Sprint(r), "invalid memory address")
		} else {
			t.Fatal("Expected panic due to nil pointer dereference")
		}
	}()

	// This should panic due to nil pointer dereference
	NewVerifier(keys, nil)
}

// Test NewVerifier with empty slices
func TestNewVerifier_EmptySlices(t *testing.T) {
	keys := []string{}
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	verifier := NewVerifier(keys, &clientInterface)

	assert.NotNil(t, verifier)
	assert.Equal(t, keys, verifier.keys)
	assert.Equal(t, clientInterface, verifier.artifactoryClient)
}

// Test NewVerifier with nil keys and keyIds but valid client and details
func TestNewVerifier_NilKeysValidClientDetails(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient

	verifier := NewVerifier(nil, &clientInterface)

	assert.NotNil(t, verifier)
	assert.Nil(t, verifier.keys)
	assert.Equal(t, clientInterface, verifier.artifactoryClient)
}

// Test Verify with nil evidence metadata
func TestVerifier_Verify_NilEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewVerifier(nil, &clientInterface)

	result, err := verifier.Verify("test-sha256", nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no evidence metadata provided")
}

// Test Verify with empty evidence metadata
func TestVerifier_Verify_EmptyEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewVerifier(nil, &clientInterface)
	emptyMetadata := &[]model.SearchEvidenceEdge{}

	result, err := verifier.Verify("test-sha256", emptyMetadata)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no evidence metadata provided")
}

// Test Verify with invalid evidence metadata (nil node)
func TestVerifier_Verify_InvalidEvidenceMetadata(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewVerifier(nil, &clientInterface)

	// Create evidence metadata with nil Subject.Sha256
	invalidMetadata := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				Subject: model.EvidenceSubject{Sha256: ""}, // Empty SHA256
			},
		},
	}

	result, err := verifier.Verify("test-sha256", invalidMetadata)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid evidence metadata")
}

// Test readEnvelopeFromRemote function directly (tests envelope reading without triggering remote key fetching)
func TestReadEnvelopeFromRemote_Success(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(createMockEnvelopeBytes())),
	}

	envelope, err := readEnvelopeFromRemote("/path/to/evidence", mockClient)

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

	envelope, err := readEnvelopeFromRemote("/path/to/evidence", mockClient)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read remote file")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test readEnvelopeFromRemote with invalid JSON
func TestReadEnvelopeFromRemote_InvalidJSON(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader([]byte("invalid json"))),
	}

	envelope, err := readEnvelopeFromRemote("/path/to/evidence", mockClient)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal envelope")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test readEnvelopeFromRemote with empty file content
func TestReadEnvelopeFromRemote_EmptyFile(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader([]byte{})),
	}

	envelope, err := readEnvelopeFromRemote("/path/to/evidence", mockClient)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal envelope")
	assert.Equal(t, dsse.Envelope{}, envelope)
}

// Test verifyEnvelop with successful verification
func TestVerifyEnvelop_SuccessfulVerification(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key",
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := createMockEnvelope()
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.True(t, success)
	assert.Equal(t, string(model.Success), result.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test verifyEnvelop with failed verification
func TestVerifyEnvelop_FailedVerification(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue:  "test-key",
		VerifyError: errors.New("verification failed"),
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockEnvelope()
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, string(model.Failed), result.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test verifyEnvelop with nil inputs
func TestVerifyEnvelop_NilInputs(t *testing.T) {
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop(nil, nil, result)

	assert.False(t, success)
	// When inputs are nil, verifyEnvelop doesn't set the status (returns early)
	assert.Equal(t, "", result.SignaturesVerificationStatus)
}

// Test verifyEnvelop with empty verifiers
func TestVerifyEnvelop_EmptyVerifiers(t *testing.T) {
	envelope := createMockEnvelope()
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop([]dsse.Verifier{}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, string(model.Failed), result.SignaturesVerificationStatus)
}

// Test verifyEnvelop with empty signatures
func TestVerifyEnvelop_EmptySignatures(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key",
	}

	envelope := dsse.Envelope{
		Payload:     "eyJ0ZXN0IjoiZGF0YSJ9",
		PayloadType: "application/vnd.in-toto+json",
		Signatures:  []dsse.Signature{}, // Empty signatures
	}
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, string(model.Failed), result.SignaturesVerificationStatus)
}

// Test verifyEnvelop with mismatched key IDs
func TestVerifyEnvelop_MismatchedKeyIDs(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "different-key", // Different from signature key ID
	}
	// Set up the mock to return an error (verification failure)
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockEnvelope() // Contains "test-key-id"
	result := &model.EvidenceVerificationResult{}

	success := verifyEnvelop([]dsse.Verifier{mockVerifier}, &envelope, result)

	assert.False(t, success)
	assert.Equal(t, string(model.Failed), result.SignaturesVerificationStatus)
	mockVerifier.AssertExpectations(t)
}

// Test getTrustedVerifiers with caching
func TestVerifier_GetTrustedVerifiers_Caching(t *testing.T) {
	cachedVerifiers := []dsse.Verifier{&MockDSSEVerifier{KeyIDValue: "cached-key"}}
	verifier := &Verifier{
		trustedVerifiers: cachedVerifiers,
	}

	verifiers, err := verifier.getTrustedVerifiers()

	assert.NoError(t, err)
	assert.Equal(t, cachedVerifiers, verifiers)
}

// Test getKeypairVerifiers with caching
func TestVerifier_GetKeypairVerifiers_Caching(t *testing.T) {
	cachedVerifiers := []dsse.Verifier{&MockDSSEVerifier{KeyIDValue: "cached-key"}}
	verifier := &Verifier{
		keypairVerifiers: cachedVerifiers,
	}

	verifiers, err := verifier.getKeypairVerifiers()

	assert.NoError(t, err)
	assert.Equal(t, cachedVerifiers, verifiers)
}
