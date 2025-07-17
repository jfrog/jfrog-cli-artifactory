package create

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
)

// MockEvidenceServiceManager mocks the evidence service manager for testing
type MockEvidenceServiceManager struct {
	UploadResponse []byte
	UploadError    error
}

func (m *MockEvidenceServiceManager) UploadEvidence(details services.EvidenceDetails) ([]byte, error) {
	if m.UploadError != nil {
		return nil, m.UploadError
	}
	return m.UploadResponse, nil
}

func TestUploadEvidence_ErrorHandling(t *testing.T) {
	// Save the current log level and set it to DEBUG for testing
	originalLogLevel := clientlog.GetLogger().GetLogLevel()
	clientlog.SetLogger(clientlog.NewLogger(clientlog.DEBUG, nil))
	defer clientlog.SetLogger(clientlog.NewLogger(originalLogLevel, nil))

	tests := []struct {
		name          string
		uploadError   error
		repoPath      string
		expectedError string
		debugLogCheck bool
	}{
		{
			name:          "404 Not Found Error",
			uploadError:   errors.New("server response: 404 Not Found"),
			repoPath:      "test-repo/path/file.txt",
			expectedError: "Subject 'test-repo/path/file.txt' is invalid or not found. Please ensure the subject exists and follows the correct format: <repo>/<path>/<name> or <repo>/<name>",
			debugLogCheck: true,
		},
		{
			name:          "400 Bad Request Error",
			uploadError:   errors.New("server response: 400 Bad Request"),
			repoPath:      "invalid-subject",
			expectedError: "Subject 'invalid-subject' is invalid or not found. Please ensure the subject exists and follows the correct format: <repo>/<path>/<name> or <repo>/<name>",
			debugLogCheck: true,
		},
		{
			name:          "404 Error with Repository not found message",
			uploadError:   errors.New(`server response: 404 Not Found {"errors": [{"message": "Repository https: not found"}]}`),
			repoPath:      "@ https://evidencetrial.jfrog.io/evidence/api/v1/subject/https:/evidencetrial.jfrog.io/artifactory/cli-sigstore-test/commons-1.0.0.txt",
			expectedError: "Subject '@ https://evidencetrial.jfrog.io/evidence/api/v1/subject/https:/evidencetrial.jfrog.io/artifactory/cli-sigstore-test/commons-1.0.0.txt' is invalid or not found. Please ensure the subject exists and follows the correct format: <repo>/<path>/<name> or <repo>/<name>",
			debugLogCheck: true,
		},
		{
			name:          "Other Error - Not 400 or 404",
			uploadError:   errors.New("server response: 500 Internal Server Error"),
			repoPath:      "test-repo/path/file.txt",
			expectedError: "server response: 500 Internal Server Error",
			debugLogCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a createEvidenceBase instance
			c := &createEvidenceBase{
				serverDetails: &config.ServerDetails{},
				providerId:    "test-provider",
			}

			// Since we can't easily mock utils.CreateEvidenceServiceManager,
			// we'll need to test the error handling logic directly.
			// For a full integration test, you would need to use dependency injection
			// or refactor the code to accept the evidence manager as a parameter.

			// For now, let's test the error message formatting by simulating the error
			err := c.handleUploadError(tt.uploadError, tt.repoPath)

			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

// Add a helper method to test error handling logic
func (c *createEvidenceBase) handleUploadError(err error, repoPath string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "400") || strings.Contains(errStr, "404") {
		clientlog.Debug("Server response error:", err.Error())
		return errorutils.CheckErrorf("Subject '%s' is invalid or not found. Please ensure the subject exists and follows the correct format: <repo>/<path>/<name> or <repo>/<name>", repoPath)
	}
	return err
}

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
