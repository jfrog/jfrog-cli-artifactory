package sigstore

import (
	"encoding/json"
	"path/filepath"
	"testing"

	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/stretchr/testify/assert"
)

func TestExtractSubjectFromRealBundle(t *testing.T) {
	bundlePath := filepath.Join("testdata", "sample-bundle.json")

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "repo/commons-1.0.0.txt", repoPath)
}

func TestExtractSubjectFromEnvelopeWithValidStatement(t *testing.T) {
	statement := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"name": "test-repo/test-artifact",
				"digest": map[string]interface{}{
					"sha256": "abcd1234567890",
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statement)
	envelope := &protodsse.Envelope{
		Payload:     payload,
		PayloadType: "application/vnd.in-toto+json",
	}

	repoPath, err := extractSubjectFromEnvelope(envelope)
	assert.NoError(t, err)
	assert.Equal(t, "test-repo/test-artifact", repoPath)
}

func TestExtractSubjectFromEnvelopeNoSubjects(t *testing.T) {
	statement := map[string]interface{}{
		"_type":         "https://in-toto.io/Statement/v1",
		"subject":       []interface{}{},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statement)
	envelope := &protodsse.Envelope{
		Payload:     payload,
		PayloadType: "application/vnd.in-toto+json",
	}

	repoPath, err := extractSubjectFromEnvelope(envelope)
	assert.NoError(t, err)
	assert.Equal(t, "", repoPath)
}

func TestExtractSubjectFromEnvelopeNoName(t *testing.T) {
	statement := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"digest": map[string]interface{}{
					"sha256": "abcd1234567890",
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statement)
	envelope := &protodsse.Envelope{
		Payload:     payload,
		PayloadType: "application/vnd.in-toto+json",
	}

	repoPath, err := extractSubjectFromEnvelope(envelope)
	assert.NoError(t, err)
	assert.Equal(t, "", repoPath)
}

func TestExtractSubjectFromEnvelopeNilEnvelope(t *testing.T) {
	repoPath, err := extractSubjectFromEnvelope(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envelope is nil")
	assert.Equal(t, "", repoPath)
}

func TestExtractSubjectFromEnvelopeInvalidJSON(t *testing.T) {
	envelope := &protodsse.Envelope{
		Payload:     []byte("invalid json"),
		PayloadType: "application/vnd.in-toto+json",
	}

	repoPath, err := extractSubjectFromEnvelope(envelope)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse statement from DSSE payload")
	assert.Equal(t, "", repoPath)
}

func TestExtractRepoPathFromStatement(t *testing.T) {
	tests := []struct {
		name      string
		statement map[string]interface{}
		expected  string
	}{
		{
			name: "valid subject with name",
			statement: map[string]interface{}{
				"subject": []interface{}{
					map[string]interface{}{
						"name": "repo/artifact",
					},
				},
			},
			expected: "repo/artifact",
		},
		{
			name: "no subjects",
			statement: map[string]interface{}{
				"subject": []interface{}{},
			},
			expected: "",
		},
		{
			name: "subject without name",
			statement: map[string]interface{}{
				"subject": []interface{}{
					map[string]interface{}{
						"digest": map[string]interface{}{"sha256": "abc123"},
					},
				},
			},
			expected: "",
		},
		{
			name: "empty name",
			statement: map[string]interface{}{
				"subject": []interface{}{
					map[string]interface{}{
						"name": "",
					},
				},
			},
			expected: "",
		},
		{
			name: "no subject field",
			statement: map[string]interface{}{
				"predicateType": "test",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepoPathFromStatement(tt.statement)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func createTestPayload(t *testing.T, statement interface{}) []byte {
	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	return statementBytes
}
