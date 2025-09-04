package verifiers

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
)

func TestParseEvidence_NilEvidence(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	parser := newEvidenceParser(mockClient, nil)

	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(nil, result)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for parsing", err.Error())
}

func TestParseEvidence_NilResult(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	parser := newEvidenceParser(mockClient, nil)

	edge := &model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}

	err := parser.parseEvidence(edge, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for parsing", err.Error())
}

func TestParseEvidence_BothNil(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	parser := newEvidenceParser(mockClient, nil)

	err := parser.parseEvidence(nil, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence or result provided for parsing", err.Error())
}

func TestTryParseSigstoreBundle_NilResult(t *testing.T) {
	parser := &evidenceParser{}
	content := createMockSigstoreBundleBytes(t)

	err := parser.tryParseSigstoreBundle(content, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty result provided for Sigstore bundle parsing", err.Error())
}

func TestTryParseDsseEnvelope_NilResult(t *testing.T) {
	parser := &evidenceParser{}
	content := createMockDsseEnvelopeBytes(t)

	err := parser.tryParseDsseEnvelope(content, nil)
	assert.Error(t, err)
	assert.Equal(t, "empty result provided for DSSE envelope parsing", err.Error())
}

func TestParseEvidence_ReadError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileError: errors.New("failed to read remote file"),
	}
	var client artifactory.ArtifactoryServicesManager = mockClient
	parser := newEvidenceParser(&client, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read remote file")
}

func TestParseEvidence_DsseEnvelope(t *testing.T) {
	dsseContent := createMockDsseEnvelopeBytes(t)
	mockClient := createMockArtifactoryClient(dsseContent)
	parser := newEvidenceParser(mockClient, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.NoError(t, err)
	assert.NotNil(t, result.DsseEnvelope)
	assert.Nil(t, result.SigstoreBundle)
	assert.Equal(t, model.SimpleDSSE, result.MediaType)
	assert.Equal(t, "eyJ0ZXN0IjoiZGF0YSJ9", result.DsseEnvelope.Payload)
}

func TestParseEvidence_SigstoreBundle(t *testing.T) {
	sigstoreContent := createMockSigstoreBundleBytes(t)
	mockClient := createMockArtifactoryClient(sigstoreContent)
	parser := newEvidenceParser(mockClient, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/sigstore/bundle",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.NoError(t, err)
	assert.NotNil(t, result.SigstoreBundle)
	assert.Nil(t, result.DsseEnvelope)
	assert.Equal(t, model.SigstoreBundle, result.MediaType)
}

func TestParseEvidence_InvalidJSON(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte("invalid json"))
	parser := newEvidenceParser(mockClient, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported evidence file for client-side verification: "+edge.Node.DownloadPath)
}

func TestParseEvidence_EmptyFile(t *testing.T) {
	mockClient := createMockArtifactoryClient([]byte{})
	parser := newEvidenceParser(mockClient, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported evidence file for client-side verification: "+edge.Node.DownloadPath)
}

func TestParseEvidence_ReadAllError(t *testing.T) {
	// Create a reader that fails on Read
	failingReader := &failingReadCloser{
		readErr: errors.New("read failed"),
	}
	mockClient := &MockArtifactoryServicesManagerVerifier{
		ReadRemoteFileResponse: failingReader,
	}
	var client artifactory.ArtifactoryServicesManager = mockClient
	parser := newEvidenceParser(&client, nil)

	edge := model.SearchEvidenceEdge{
		Node: model.EvidenceMetadata{
			DownloadPath: "/path/to/evidence",
		},
	}
	result := &model.EvidenceVerification{}

	err := parser.parseEvidence(&edge, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file content")
}

// Helper type for testing io.ReadAll failures
type failingReadCloser struct {
	readErr error
}

func (f *failingReadCloser) Read(p []byte) (n int, err error) {
	return 0, f.readErr
}

func (f *failingReadCloser) Close() error {
	return nil
}
