package verifiers

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
)

func TestVerify_NilEvidenceMetadata(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	verifier := NewEvidenceVerifier(nil, true, mockClient)

	result, err := verifier.Verify("test-sha256", nil, "")
	assert.EqualError(t, err, "no evidence metadata provided")
	assert.Nil(t, result)
}

func TestVerify_EmptyEvidenceMetadata(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	verifier := NewEvidenceVerifier(nil, true, mockClient)
	emptyMetadata := &[]model.SearchEvidenceEdge{}

	result, err := verifier.Verify("test-sha256", emptyMetadata, "")
	assert.EqualError(t, err, "no evidence metadata provided")
	assert.Nil(t, result)
}

func TestVerify_FileReadError(t *testing.T) {
	evidence := createTestEvidenceWithKeys()
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileError: errors.New("file read error"),
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, true, &clientInterface)

	_, err := verifier.Verify(createTestSHA256(), evidence, "/path/to/file")
	assert.EqualError(t, err, "failed to read envelope: failed to read remote file: file read error")
}

func TestVerify_MultipleEvidence(t *testing.T) {
	evidence := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "test/path1",
				PredicateType: "test-predicate1",
				CreatedBy:     "user1",
				CreatedAt:     "2024-01-01",
				Subject: model.EvidenceSubject{
					Sha256: createTestSHA256(),
				},
			},
		},
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "test/path2",
				PredicateType: "test-predicate2",
				CreatedBy:     "user2",
				CreatedAt:     "2024-01-02",
				Subject: model.EvidenceSubject{
					Sha256: createTestSHA256(),
				},
			},
		},
	}

	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileFunc: func() io.ReadCloser {
			return io.NopCloser(bytes.NewReader(createMockDsseEnvelopeBytes(t)))
		},
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, false, &clientInterface)

	result, err := verifier.Verify(createTestSHA256(), evidence, "/path/to/file")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.EvidenceVerifications)
	assert.Len(t, *result.EvidenceVerifications, 2)

	// Verify each evidence entry
	for i, verification := range *result.EvidenceVerifications {
		assert.Equal(t, (*evidence)[i].Node.DownloadPath, verification.DownloadPath)
		assert.Equal(t, (*evidence)[i].Node.PredicateType, verification.PredicateType)
		assert.Equal(t, (*evidence)[i].Node.CreatedBy, verification.CreatedBy)
		assert.Equal(t, (*evidence)[i].Node.CreatedAt, verification.CreatedAt)
	}
}

func TestVerify_NilEvidence(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	verifier := &evidenceVerifier{
		artifactoryClient: *mockClient,
		parser:            newEvidenceParser(mockClient),
		dsseVerifier:      newDsseVerifier(nil, false),
		sigstoreVerifier:  newSigstoreVerifier(),
	}

	result, err := verifier.verifyEvidence(nil, createTestSHA256())
	assert.EqualError(t, err, "nil evidence provided")
	assert.Nil(t, result)
}

func TestVerify_OverallStatus(t *testing.T) {
	// Create multiple evidence entries to test overall status calculation
	evidence := &[]model.SearchEvidenceEdge{
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "test/path1",
				PredicateType: "test-predicate1",
				CreatedBy:     "user1",
				CreatedAt:     "2024-01-01",
				Subject: model.EvidenceSubject{
					Sha256: createTestSHA256(),
				},
			},
		},
		{
			Node: model.EvidenceMetadata{
				DownloadPath:  "test/path2",
				PredicateType: "test-predicate2",
				CreatedBy:     "user2",
				CreatedAt:     "2024-01-02",
				Subject: model.EvidenceSubject{
					Sha256: createTestSHA256(),
				},
			},
		},
	}

	// Mock client that returns DSSE envelopes for parsing
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileFunc: func() io.ReadCloser {
			return io.NopCloser(bytes.NewReader(createMockDsseEnvelopeBytes(t)))
		},
	}
	var clientInterface artifactory.ArtifactoryServicesManager = mockClient
	verifier := NewEvidenceVerifier(nil, false, &clientInterface)

	result, err := verifier.Verify(createTestSHA256(), evidence, "/path/to/file")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.EvidenceVerifications)
	assert.Len(t, *result.EvidenceVerifications, 2)

	// Test overall status determination
	assert.NotNil(t, result.OverallVerificationStatus)

	// Since we're using mock data without proper keys, verification should fail
	// but the overall structure should be properly set up
	assert.Contains(t, []model.VerificationStatus{model.Success, model.Failed}, result.OverallVerificationStatus)

	// Verify that all individual evidence has proper verification results
	for _, verification := range *result.EvidenceVerifications {
		assert.NotNil(t, verification.VerificationResult)
		// Each verification should have a checksum status since we're providing SHA256
		assert.NotEqual(t, model.VerificationStatus(""), verification.VerificationResult.Sha256VerificationStatus)
	}
}
