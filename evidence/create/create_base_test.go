package create

import (
	"encoding/json"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndSignEnvelope(t *testing.T) {
	tests := []struct {
		name             string
		payloadJson      []byte
		keyPath          string
		keyId            string
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:        "Valid ECDSA key",
			payloadJson: []byte(`{"foo": "bar"}`),
			keyPath:     "tests/testdata/ecdsa_key.pem",
			keyId:       "test-key-id",
			expectError: false,
		},
		{
			name:             "Unsupported key type",
			payloadJson:      []byte(`{"foo": "bar"}`),
			keyPath:          "tests/testdata/unsupported_key.pem",
			keyId:            "test-key-id",
			expectError:      true,
			expectedErrorMsg: "failed to decode the data as PEM block (are you sure this is a pem file?)",
		},
		{
			name:             "public key type",
			payloadJson:      []byte(`{"foo": "bar"}`),
			keyPath:          "tests/testdata/public_key.pem",
			keyId:            "test-key-id",
			expectError:      true,
			expectedErrorMsg: "failed to load private key. please verify provided key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyContent, err := os.ReadFile(filepath.Join("../..", tt.keyPath))

			if err != nil {
				t.Fatalf("failed to read key file: %v", err)
			}

			envelope, err := createAndSignEnvelope(tt.payloadJson, string(keyContent), tt.keyId)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, envelope)
				assert.Equal(t, tt.expectedErrorMsg, err.Error())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, envelope)

				envelopeBytes, _ := json.Marshal(envelope)

				var signedEnvelope dsse.Envelope
				err = json.Unmarshal(envelopeBytes, &signedEnvelope)
				assert.NoError(t, err)
				assert.Equal(t, intoto.PayloadType, signedEnvelope.PayloadType)
			}
		})
	}
}
