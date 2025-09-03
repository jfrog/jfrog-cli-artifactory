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
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
)

type mockServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockServicesManager) FileInfo(relativePath string) (*utils.FileInfo, error) {
	if relativePath == "exists" {
		return &utils.FileInfo{Checksums: struct {
			Sha1   string `json:"sha1,omitempty"`
			Sha256 string `json:"sha256,omitempty"`
			Md5    string `json:"md5,omitempty"`
		}{Sha256: "abc"}}, nil
	}
	return nil, errors.New("not found")
}

func TestResolveSubjectSha256_MatchProvided(t *testing.T) {
	c := &createEvidenceBase{}
	got, err := c.resolveSubjectSha256(&mockServicesManager{}, "exists", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc" {
		t.Fatalf("expected abc got %s", got)
	}
}

func TestResolveSubjectSha256_Mismatch(t *testing.T) {
	c := &createEvidenceBase{}
	_, err := c.resolveSubjectSha256(&mockServicesManager{}, "exists", "mismatch")
	if err == nil {
		t.Fatalf("expected error on mismatch")
	}
}

func TestResolveSubjectSha256_NoProvidedUsesFetched(t *testing.T) {
	c := &createEvidenceBase{}
	got, err := c.resolveSubjectSha256(&mockServicesManager{}, "exists", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc" {
		t.Fatalf("expected fetched sha abc got %s", got)
	}
}

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
	originalLogLevel := log.GetLogger().GetLogLevel()
	log.SetLogger(log.NewLogger(log.DEBUG, nil))
	defer log.SetLogger(log.NewLogger(originalLogLevel, nil))

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
			c := &createEvidenceBase{
				serverDetails: &config.ServerDetails{},
				providerId:    "test-integration",
			}
			err := c.handleUploadError(tt.uploadError, tt.repoPath)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())
		})
	}
}

// test helper
func (c *createEvidenceBase) handleUploadError(err error, repoPath string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "400") || strings.Contains(errStr, "404") {
		log.Debug("Server response error:", err.Error())
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
			expectedErrorMsg: "key pair is incorrect or key alias 'test-key-id' was not found in Artifactory. Original error: failed to decode the data as PEM block (are you sure this is a pem file?)",
		},
		{
			name:             "public key type",
			payloadJson:      []byte(`{"foo": "bar"}`),
			keyPath:          "tests/testdata/public_key.pem",
			keyId:            "test-key-id",
			expectError:      true,
			expectedErrorMsg: "key pair is incorrect or key alias 'test-key-id' was not found in Artifactory. Original error: failed to load private key",
		},
		{
			name:             "public key type without keyId",
			payloadJson:      []byte(`{"foo": "bar"}`),
			keyPath:          "tests/testdata/public_key.pem",
			keyId:            "",
			expectError:      true,
			expectedErrorMsg: "failed to load private key. Please verify the provided key is correct or check if the key alias exists in Artifactory. Original error: failed to load private key",
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

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_NotInGitHub(t *testing.T) {
	assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
	base := &createEvidenceBase{predicateType: "https://slsa.dev/provenance/v1", providerId: "test-integration"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/test/subject", Verified: true, PredicateType: base.predicateType}
	err := base.recordEvidenceSummary(summaryData)
	assert.NoError(t, err)
}

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_GitHubCommiter(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, fileutils.RemoveTempDir(tempDir)) }()
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()
	base := &createEvidenceBase{predicateType: "https://slsa.dev/provenance/v1", providerId: "test-integration", flagType: "gh-commiter"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/test/subject", Verified: true, PredicateType: base.predicateType}
	err = base.recordEvidenceSummary(summaryData)
	assert.NoError(t, err)
}

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_Success(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, fileutils.RemoveTempDir(tempDir)) }()
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()
	base := &createEvidenceBase{predicateType: "https://slsa.dev/provenance/v1", providerId: "test-integration", flagType: "other"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/docker-cosign-test/hello-world", Verified: true, PredicateType: base.predicateType}
	err = base.recordEvidenceSummary(summaryData)
	assert.NoError(t, err)
}

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_NoSummaryEnv(t *testing.T) {
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	defer func() { assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS")) }()
	base := &createEvidenceBase{predicateType: "https://slsa.dev/provenance/v1", providerId: "test-integration", flagType: "other"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/test/subject", Verified: true, PredicateType: base.predicateType}
	err := base.recordEvidenceSummary(summaryData)
	assert.Error(t, err)
}

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_Verified(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, fileutils.RemoveTempDir(tempDir)) }()
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()
	base := &createEvidenceBase{predicateType: "", providerId: "test-integration", flagType: "other"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/docker-cosign-test/hello-world", Verified: true, PredicateType: base.predicateType}
	err = base.recordEvidenceSummary(summaryData)
	assert.NoError(t, err)
}

func TestCreateEvidenceBase_RecordEvidenceSummaryIfInGitHubActions_NotVerified(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, fileutils.RemoveTempDir(tempDir)) }()
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()
	base := &createEvidenceBase{predicateType: "", providerId: "test-integration", flagType: "other"}
	summaryData := commandsummary.EvidenceSummaryData{Subject: "/test-repo/artifact", Verified: false, PredicateType: base.predicateType}
	err = base.recordEvidenceSummary(summaryData)
	assert.NoError(t, err)
}
