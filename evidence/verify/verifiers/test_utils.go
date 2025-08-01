package verifiers

import (
	"bytes"
	"crypto"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/mock"
)

func createTestSHA256() string {
	return "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}

func createTestEvidenceWithKeys() *[]model.SearchEvidenceEdge {
	return &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "test/path",
				PredicateType: "test-predicate",
				CreatedBy:     "test-user",
				CreatedAt:     "2024-01-01",
				Subject: model.EvidenceSubject{
					Sha256: createTestSHA256(),
				},
				SigningKey: model.SingingKey{
					PublicKey: "test-public-key",
				},
			},
		},
	}
}

func createMockDsseEnvelope() dsse.Envelope {
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

func createMockDsseEnvelopeBytes(t *testing.T) []byte {
	envelope := createMockDsseEnvelope()
	data, err := json.Marshal(envelope)
	if err != nil {
		assert.Fail(t, "Failed to marshal DSSE envelope", err)
	}
	return data
}

func createMockSigstoreBundleBytes(t *testing.T) []byte {
	sigstoreBundle := map[string]interface{}{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": map[string]interface{}{
			"certificate": map[string]interface{}{
				"rawBytes": "dGVzdC1jZXJ0",
			},
		},
		"dsseEnvelope": map[string]interface{}{
			"payload":     "eyJ0ZXN0IjoiZGF0YSJ9",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": []map[string]interface{}{
				{
					"sig":   "dGVzdC1zaWduYXR1cmU=",
					"keyid": "test-key-id",
				},
			},
		},
	}

	data, err := json.Marshal(sigstoreBundle)
	if err != nil {
		assert.Fail(t, "Failed to marshal Sigstore bundle", err)
	}
	return data
}

// Mock implementations
type MockArtifactoryServicesManagerVerifier struct {
	artifactory.EmptyArtifactoryServicesManager
	ReadRemoteFileResponse io.ReadCloser
	ReadRemoteFileError    error
	ReadRemoteFileFunc     func() io.ReadCloser // Function to create new readers for each call
}

func (m *MockArtifactoryServicesManagerVerifier) ReadRemoteFile(_ string) (io.ReadCloser, error) {
	if m.ReadRemoteFileError != nil {
		return nil, m.ReadRemoteFileError
	}
	if m.ReadRemoteFileFunc != nil {
		return m.ReadRemoteFileFunc(), nil
	}
	return m.ReadRemoteFileResponse, nil
}

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

// MockEnvelope mocks the dsse.Envelope struct for testing purposes
type MockEnvelope struct {
	mock.Mock
}

func (m *MockEnvelope) Verify(verifiers ...dsse.Verifier) error {
	args := m.Called(verifiers)
	return args.Error(0)
}

// MockTUFRootCertificateProvider mocks the TUF root certificate provider to avoid real network calls in tests
type MockTUFRootCertificateProvider struct {
	mock.Mock
}

func (m *MockTUFRootCertificateProvider) LoadTUFRootCertificate() (root.TrustedMaterial, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if trustedMaterial, ok := args.Get(0).(root.TrustedMaterial); ok {
		return trustedMaterial, args.Error(1)
	}
	return nil, args.Error(1)
}

// Helper to create mock artifactory client
func createMockArtifactoryClient(fileContent []byte) *artifactory.ArtifactoryServicesManager {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: io.NopCloser(bytes.NewReader(fileContent)),
	}
	var client artifactory.ArtifactoryServicesManager = mockClient
	return &client
}
