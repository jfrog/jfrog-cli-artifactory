package verifiers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDsseVerifier_Verify_NilEvidence(t *testing.T) {
	verifier := &dsseVerifier{
		keys:               []string{},
		useArtifactoryKeys: false,
	}

	result := &model.EvidenceVerification{
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(createTestSHA256(), nil, result)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for DSSE verification", err.Error())
}

func TestDsseVerifier_Verify_NilResult(t *testing.T) {
	verifier := &dsseVerifier{
		keys:               []string{},
		useArtifactoryKeys: false,
	}

	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: createTestSHA256(),
			},
		},
	}

	err := verifier.verify(createTestSHA256(), evidence, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for DSSE verification", err.Error())
}

func TestDsseVerifier_Verify_BothNil(t *testing.T) {
	verifier := &dsseVerifier{
		keys:               []string{},
		useArtifactoryKeys: false,
	}

	err := verifier.verify(createTestSHA256(), nil, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for DSSE verification", err.Error())
}

func TestDsseVerifier_VerifyWithLocalKeys_Success(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
		PublicKey:  &rsa.PublicKey{N: big.NewInt(1), E: 65537}, // Minimal valid RSA public key
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := dsse.Envelope{
		Payload:     base64.StdEncoding.EncodeToString([]byte("test-payload")),
		PayloadType: "application/vnd.in-toto+json",
		Signatures: []dsse.Signature{
			{
				KeyId: "test-key-id",
				Sig:   base64.StdEncoding.EncodeToString([]byte("test-signature")),
			},
		},
	}

	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: createTestSHA256(),
			},
			SigningKey: model.SingingKey{},
		},
	}

	verifier := &dsseVerifier{
		keys:               []string{"test-key"},
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{mockVerifier},
	}

	err := verifier.verify(createTestSHA256(), evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.Sha256VerificationStatus)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.SignaturesVerificationStatus)
	assert.Equal(t, localKeySource, result.VerificationResult.KeySource)

	mockVerifier.AssertExpectations(t)
}

func TestDsseVerifier_VerifyWithLocalKeys_Failed(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
		PublicKey:  &rsa.PublicKey{N: big.NewInt(1), E: 65537},
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: createTestSHA256(),
			},
			SigningKey: model.SingingKey{},
		},
	}

	verifier := &dsseVerifier{
		keys:               []string{"test-key"},
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{mockVerifier},
	}

	err := verifier.verify(createTestSHA256(), evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.Sha256VerificationStatus)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

func TestDsseVerifier_VerifyWithMultipleLocalKeys(t *testing.T) {
	mockVerifier1 := &MockDSSEVerifier{
		KeyIDValue: "key-1",
		PublicKey:  &rsa.PublicKey{N: big.NewInt(1), E: 65537},
	}
	mockVerifier1.On("Verify", mock.Anything, mock.Anything).Return(errors.New("wrong key"))

	mockVerifier2 := &MockDSSEVerifier{
		KeyIDValue: "key-2",
		PublicKey:  &rsa.PublicKey{N: big.NewInt(2), E: 65537},
	}
	mockVerifier2.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := dsse.Envelope{
		Payload:     base64.StdEncoding.EncodeToString([]byte("test-payload")),
		PayloadType: "application/vnd.in-toto+json",
		Signatures: []dsse.Signature{
			{
				KeyId: "key-2",
				Sig:   base64.StdEncoding.EncodeToString([]byte("test-signature")),
			},
		},
	}

	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: createTestSHA256(),
			},
			SigningKey: model.SingingKey{},
		},
	}

	verifier := &dsseVerifier{
		keys:               []string{"key1", "key2"},
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{mockVerifier1, mockVerifier2},
	}

	err := verifier.verify(createTestSHA256(), evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.Sha256VerificationStatus)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.SignaturesVerificationStatus)
	assert.Equal(t, localKeySource, result.VerificationResult.KeySource)
}

func TestDsseVerifier_VerifyWithLocalKeys(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key-id",
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}
	evidence := &model.SearchEvidenceEdge{}

	verifier := &dsseVerifier{
		keys:               []string{"test-key"},
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{mockVerifier},
	}

	err := verifier.verify("", evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.SignaturesVerificationStatus)
	assert.Equal(t, localKeySource, result.VerificationResult.KeySource)
}

func TestDsseVerifier_VerifyNoKeysAvailable(t *testing.T) {
	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}
	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			SigningKey: model.SingingKey{}, // No public key
		},
	}

	verifier := &dsseVerifier{
		useArtifactoryKeys: true,
		localKeys:          []dsse.Verifier{}, // No local keys
	}

	err := verifier.verify("", evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

func TestDsseVerifier_GetLocalVerifiers(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test-key.pem")

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publicKeyPEM := exportRSAPublicKeyAsPEM(&privateKey.PublicKey)

	err := os.WriteFile(keyPath, publicKeyPEM, 0644)
	assert.NoError(t, err)

	verifier := &dsseVerifier{
		keys: []string{keyPath},
	}

	verifiers, err := verifier.getLocalVerifiers()
	assert.NoError(t, err)
	assert.Len(t, verifiers, 1)
	assert.NotNil(t, verifiers[0])
}

func TestDsseVerifier_GetLocalVerifiersFileNotFound(t *testing.T) {
	verifier := &dsseVerifier{
		keys: []string{"/non/existent/key.pem"},
	}

	verifiers, err := verifier.getLocalVerifiers()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read key")
	assert.Nil(t, verifiers)
}

func TestDsseVerifier_GetLocalVerifiersEmptyKeyPath(t *testing.T) {
	verifier := &dsseVerifier{
		keys: []string{""},
	}

	verifiers, err := verifier.getLocalVerifiers()
	assert.NoError(t, err)
	assert.Len(t, verifiers, 0)
}

func TestVerifyEnvelope_Success(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue: "test-key",
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		VerificationResult: model.EvidenceVerificationResult{},
	}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)
	assert.True(t, success)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.SignaturesVerificationStatus)
}

func TestVerifyEnvelope_Failed(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{
		KeyIDValue:  "test-key",
		VerifyError: errors.New("verification failed"),
	}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(errors.New("verification failed"))

	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		VerificationResult: model.EvidenceVerificationResult{},
	}

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, result)
	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

func TestVerifyEnvelope_NilInputs(t *testing.T) {
	result := &model.EvidenceVerification{
		VerificationResult: model.EvidenceVerificationResult{},
	}

	success := verifyEnvelope(nil, nil, result)
	assert.False(t, success)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.SignaturesVerificationStatus)
}

func TestVerifyEnvelope_NilResult(t *testing.T) {
	mockVerifier := &MockDSSEVerifier{}
	mockVerifier.On("Verify", mock.Anything, mock.Anything).Return(nil)

	envelope := createMockDsseEnvelope()

	success := verifyEnvelope([]dsse.Verifier{mockVerifier}, &envelope, nil)
	assert.False(t, success)
}

func TestGetArtifactoryVerifiers_NilEvidence(t *testing.T) {
	verifiers, err := getArtifactoryVerifiers(nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence provided for artifactory verifier retrieval", err.Error())
	assert.Nil(t, verifiers)
}

func TestGetArtifactoryVerifiers_Success(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publicKeyPEM := exportRSAPublicKeyAsPEM(&privateKey.PublicKey)

	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			SigningKey: model.SingingKey{
				PublicKey: string(publicKeyPEM),
			},
		},
	}

	verifiers, err := getArtifactoryVerifiers(evidence)
	assert.NoError(t, err)
	assert.NotEmpty(t, verifiers)
}

func TestGetArtifactoryVerifiers_NoPublicKey(t *testing.T) {
	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			SigningKey: model.SingingKey{},
		},
	}

	verifiers, err := getArtifactoryVerifiers(evidence)
	assert.NoError(t, err)
	assert.Empty(t, verifiers)
}

func TestGetArtifactoryVerifiers_InvalidKey(t *testing.T) {
	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			SigningKey: model.SingingKey{
				PublicKey: "invalid-key-data",
			},
		},
	}

	_, err := getArtifactoryVerifiers(evidence)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load artifactory key")
}

func TestDsseVerifier_ChecksumMismatch(t *testing.T) {
	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	// Evidence with different SHA256 than subject
	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			},
			SigningKey: model.SingingKey{}, // No public key
		},
	}

	verifier := &dsseVerifier{
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{}, // No local keys
	}

	// Use different SHA256 for subject to trigger checksum mismatch
	subjectSha256 := createTestSHA256()

	err := verifier.verify(subjectSha256, evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Failed), result.VerificationResult.Sha256VerificationStatus)
}

func TestDsseVerifier_ChecksumMatch(t *testing.T) {
	envelope := createMockDsseEnvelope()
	result := &model.EvidenceVerification{
		DsseEnvelope:       &envelope,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	// Evidence with same SHA256 as subject
	sha256 := createTestSHA256()
	evidence := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			Subject: model.EvidenceSubject{
				Sha256: sha256,
			},
			SigningKey: model.SingingKey{}, // No public key
		},
	}

	verifier := &dsseVerifier{
		useArtifactoryKeys: false,
		localKeys:          []dsse.Verifier{}, // No local keys
	}

	err := verifier.verify(sha256, evidence, result)
	assert.NoError(t, err)
	assert.Equal(t, model.VerificationStatus(model.Success), result.VerificationResult.Sha256VerificationStatus)
}

func TestVerifyChecksum_Success(t *testing.T) {
	sha256 := createTestSHA256()
	result := verifyChecksum(sha256, sha256)
	assert.Equal(t, model.VerificationStatus(model.Success), result)
}

func TestVerifyChecksum_Failed(t *testing.T) {
	sha256a := createTestSHA256()
	sha256b := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	result := verifyChecksum(sha256a, sha256b)
	assert.Equal(t, model.VerificationStatus(model.Failed), result)
}

// Helper function to export RSA public key as PEM
func exportRSAPublicKeyAsPEM(pubkey *rsa.PublicKey) []byte {
	pubKeyBytes, _ := x509.MarshalPKIXPublicKey(pubkey)
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	return pem.EncodeToMemory(pemBlock)
}
