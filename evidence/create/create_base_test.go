package create

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	evdservices "github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"

	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	cryptox "github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
)

func createMockServicesManagerForSha256Tests() *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		FileInfoFunc: func(relativePath string) (*utils.FileInfo, error) {
			if relativePath == "exists" {
				return NewFileInfoBuilder().
					WithPath(relativePath).
					WithSha256("abc").
					Build(), nil
			}
			return nil, errors.New("not found")
		},
	}
}

func TestResolveSubjectSha256_MatchProvided(t *testing.T) {
	c := &createEvidenceBase{}
	mockManager := createMockServicesManagerForSha256Tests()
	got, err := c.resolveSubjectSha256(mockManager, "exists", "abc")
	assert.NoError(t, err, "resolveSubjectSha256 should not return error")
	assert.Equal(t, "abc", got, "should return expected sha256")
}

func TestResolveSubjectSha256_Mismatch(t *testing.T) {
	c := &createEvidenceBase{}
	mockManager := createMockServicesManagerForSha256Tests()
	_, err := c.resolveSubjectSha256(mockManager, "exists", "mismatch")
	assert.Error(t, err, "should return error on sha256 mismatch")
	assert.Contains(t, err.Error(), "provided sha256 does not match", "error should indicate mismatch")
}

func TestResolveSubjectSha256_NoProvidedUsesFetched(t *testing.T) {
	c := &createEvidenceBase{}
	mockManager := createMockServicesManagerForSha256Tests()
	got, err := c.resolveSubjectSha256(mockManager, "exists", "")
	assert.NoError(t, err, "should not return error when using fetched sha256")
	assert.Equal(t, "abc", got, "should return fetched sha256 when none provided")
}

// MockEvidenceServiceManager mocks the evidence service manager for testing
type MockEvidenceServiceManager struct {
	UploadResponse []byte
	UploadError    error
}

func (m *MockEvidenceServiceManager) UploadEvidence(details evdservices.EvidenceDetails) ([]byte, error) {
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
			assert.NoError(t, err, "should be able to read key file")
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

type captureUploader struct{ last evdservices.EvidenceDetails }

func (c *captureUploader) UploadEvidence(d evdservices.EvidenceDetails) ([]byte, error) {
	resp := model.CreateResponse{PredicateSlug: "slug", Verified: true, PredicateType: "t"}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	c.last = d
	return b, nil
}

// createFileInfoOnlyMock creates a mock that returns FileInfo with specified SHA256
func createFileInfoOnlyMock(sha string) *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		FileInfoFunc: func(_ string) (*utils.FileInfo, error) {
			return NewFileInfoBuilder().
				WithSha256(sha).
				Build(), nil
		},
	}
}

type fakeStmtResolver struct {
	out []byte
	err error
}

func (f *fakeStmtResolver) ResolveStatement() ([]byte, error) { return f.out, f.err }

func TestCreateEnvelope_Intoto_AndUploadDetails(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "predicate.json")
	md := filepath.Join(dir, "notes.md")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"k":"v"}`), 0600))
	assert.NoError(t, os.WriteFile(md, []byte("hello"), 0600))

	keyContent, err := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	assert.NoError(t, err)

	art := createFileInfoOnlyMock("abc123")
	upl := &captureUploader{}

	c := &createEvidenceBase{
		serverDetails:     &config.ServerDetails{User: "alice"},
		predicateFilePath: pred,
		predicateType:     "test-type",
		markdownFilePath:  md,
		key:               string(keyContent),
		artifactoryClient: art,
		uploader:          upl,
		providerId:        "prov",
	}

	env, err := c.createEnvelope("repo/path/name", "")
	assert.NoError(t, err)

	var envObj dsse.Envelope
	assert.NoError(t, json.Unmarshal(env, &envObj))
	assert.Equal(t, intoto.PayloadType, envObj.PayloadType)

	decoded, err := base64.StdEncoding.DecodeString(envObj.Payload)
	assert.NoError(t, err)
	var st intoto.Statement
	assert.NoError(t, json.Unmarshal(decoded, &st))
	assert.Equal(t, "test-type", st.PredicateType)
	assert.Equal(t, "abc123", st.Subject[0].Digest.Sha256)
	assert.Equal(t, "hello", st.Markdown)
	assert.Equal(t, "alice", st.CreatedBy)

	resp, err := c.uploadEvidence(env, "repo/path/name")
	assert.NoError(t, err)
	assert.True(t, resp.Verified)
	assert.Equal(t, "repo/path/name", upl.last.SubjectUri)
	assert.Equal(t, env, upl.last.DSSEFileRaw)
	assert.Equal(t, "prov", upl.last.ProviderId)
}

func TestCreateEnvelope_Sonar_ProviderAndSubjectStage(t *testing.T) {
	keyContent, err := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	assert.NoError(t, err)

	art := createFileInfoOnlyMock("deadbeef")
	stmt := []byte(`{"predicateType":"x"}`)

	c := &createEvidenceBase{
		serverDetails:     &config.ServerDetails{User: "bob"},
		key:               string(keyContent),
		stage:             "promoted",
		integration:       "sonar",
		artifactoryClient: art,
		stmtResolver:      &fakeStmtResolver{out: stmt},
	}

	env, err := c.createEnvelope("any/repo/path", "")
	assert.NoError(t, err)
	assert.Equal(t, "sonar", c.providerId)

	var envObj dsse.Envelope
	assert.NoError(t, json.Unmarshal(env, &envObj))
	decoded, err := base64.StdEncoding.DecodeString(envObj.Payload)
	assert.NoError(t, err)
	var payload map[string]any
	assert.NoError(t, json.Unmarshal(decoded, &payload))
	// subject and stage added
	assert.Equal(t, "promoted", payload["stage"])
	subj, ok := payload["subject"].([]any)
	assert.True(t, ok)
	first, ok := subj[0].(map[string]any)
	assert.True(t, ok)
	dig, ok := first["digest"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "deadbeef", dig["sha256"])
}

func TestCreateArtifactoryClient_UsesInjected(t *testing.T) {
	art := &SimpleMockServicesManager{}
	c := &createEvidenceBase{artifactoryClient: art}
	got, err := c.createArtifactoryClient()
	assert.NoError(t, err)
	assert.Equal(t, art, got)
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

type failingUploader struct {
	last evdservices.EvidenceDetails
	err  error
}

func (f *failingUploader) UploadEvidence(d evdservices.EvidenceDetails) ([]byte, error) {
	f.last = d
	return nil, f.err
}

type invalidJSONUploader struct{ last evdservices.EvidenceDetails }

func (i *invalidJSONUploader) UploadEvidence(d evdservices.EvidenceDetails) ([]byte, error) {
	i.last = d
	return []byte("x"), nil
}

func TestBuildIntotoStatementJson_PredicateReadError(t *testing.T) {
	c := &createEvidenceBase{predicateFilePath: "/no/such/file.json", serverDetails: &config.ServerDetails{}}
	_, err := c.buildIntotoStatementJson("repo/p", "")
	assert.Error(t, err)
}

func TestSetMarkdown_ExtensionError(t *testing.T) {
	c := &createEvidenceBase{markdownFilePath: "file.txt"}
	err := c.setMarkdown(&intoto.Statement{})
	assert.Error(t, err)
}

func TestSetMarkdown_ReadError(t *testing.T) {
	c := &createEvidenceBase{markdownFilePath: "file.md"}
	err := c.setMarkdown(&intoto.Statement{})
	assert.Error(t, err)
}

func TestResolveSubjectSha256_FileInfoError(t *testing.T) {
	c := &createEvidenceBase{}
	mockManager := createMockServicesManagerForSha256Tests()
	_, err := c.resolveSubjectSha256(mockManager, "missing", "")
	assert.Error(t, err)
}

func TestCreateEnvelope_SubjectShaMismatch(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBase{predicateFilePath: pred, predicateType: "t", key: string(keyContent), serverDetails: &config.ServerDetails{}, artifactoryClient: createFileInfoOnlyMock("abc")}
	_, err := c.createEnvelope("repo/a", "def")
	assert.Error(t, err)
}

func TestUploadEvidence_Error(t *testing.T) {
	u := &failingUploader{err: errors.New("server response: 500 Internal Server Error")}
	c := &createEvidenceBase{uploader: u}
	_, err := c.uploadEvidence([]byte("data"), "r/p")
	assert.Error(t, err)
}

func TestUploadEvidence_InvalidJSON(t *testing.T) {
	u := &invalidJSONUploader{}
	c := &createEvidenceBase{uploader: u}
	_, err := c.uploadEvidence([]byte("data"), "r/p")
	assert.Error(t, err)
}

func TestAddSubjectAndStageToStatement_StageEmpty(t *testing.T) {
	in := []byte(`{"predicateType":"x"}`)
	out, err := addSubjectAndStageToStatement(in, "abc", "")
	assert.NoError(t, err)
	var m map[string]any
	assert.NoError(t, json.Unmarshal(out, &m))
	_, hasStage := m["stage"]
	assert.False(t, hasStage)
	subj, ok := m["subject"].([]any)
	assert.True(t, ok)
	firstSubj, ok := subj[0].(map[string]any)
	assert.True(t, ok)
	dg, ok := firstSubj["digest"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "abc", dg["sha256"])
}

func TestBuildSonarStatement_ResolverError(t *testing.T) {
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBase{stmtResolver: &fakeStmtResolver{err: errors.New("x")}, artifactoryClient: createFileInfoOnlyMock("s"), key: string(keyContent)}
	_, err := c.buildSonarStatement("r/p", "")
	assert.Error(t, err)
}

func TestBuildSonarStatement_InvalidJSON(t *testing.T) {
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBase{stmtResolver: &fakeStmtResolver{out: []byte("x")}, artifactoryClient: createFileInfoOnlyMock("s"), key: string(keyContent)}
	_, err := c.buildSonarStatement("r/p", "")
	assert.Error(t, err)
}

func TestBuildIntotoStatementJsonWithPredicateAndType(t *testing.T) {
	art := createFileInfoOnlyMock("zzz")
	c := &createEvidenceBase{serverDetails: &config.ServerDetails{User: "bob"}, artifactoryClient: art}
	out, err := c.buildIntotoStatementJsonWithPredicateAndPredicateType("r/p", "", "ptype", []byte(`{"a":2}`))
	assert.NoError(t, err)
	var st intoto.Statement
	assert.NoError(t, json.Unmarshal(out, &st))
	assert.Equal(t, "ptype", st.PredicateType)
	assert.Equal(t, "zzz", st.Subject[0].Digest.Sha256)
}

func TestBuildIntotoStatementJsonWithPredicateAndType_MarkdownError(t *testing.T) {
	art := createFileInfoOnlyMock("sha")
	c := &createEvidenceBase{serverDetails: &config.ServerDetails{User: "u"}, artifactoryClient: art, markdownFilePath: "bad.txt"}
	_, err := c.buildIntotoStatementJsonWithPredicateAndPredicateType("r/p", "", "ptype", []byte(`{"a":2}`))
	assert.Error(t, err)
}

func TestBuildIntotoStatementJson_ResolveSubjectSha256Error(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	c := &createEvidenceBase{serverDetails: &config.ServerDetails{User: "u"}, predicateFilePath: pred, predicateType: "ptype", artifactoryClient: &failingArt{}}
	_, err := c.buildIntotoStatementJson("r/p", "")
	assert.Error(t, err)
}

func TestCreateEnvelope_DefaultUserWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createFileInfoOnlyMock("sha")
	c := &createEvidenceBase{predicateFilePath: pred, predicateType: "ptype", key: string(keyContent), artifactoryClient: art, serverDetails: &config.ServerDetails{User: ""}}
	env, err := c.createEnvelope("r/p", "")
	assert.NoError(t, err)
	var ds dsse.Envelope
	assert.NoError(t, json.Unmarshal(env, &ds))
	decoded, _ := base64.StdEncoding.DecodeString(ds.Payload)
	var st intoto.Statement
	assert.NoError(t, json.Unmarshal(decoded, &st))
	assert.Equal(t, EvdDefaultUser, st.CreatedBy)
}

func TestCreateEnvelopeWithPredicateAndPredicateType_SetsFields(t *testing.T) {
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := createFileInfoOnlyMock("ssss")
	c := &createEvidenceBase{key: string(keyContent), artifactoryClient: art, serverDetails: &config.ServerDetails{User: "u"}}
	env, err := c.createEnvelopeWithPredicateAndPredicateType("r/p", "", "my-type", []byte(`{"z":3}`))
	assert.NoError(t, err)
	var ds dsse.Envelope
	assert.NoError(t, json.Unmarshal(env, &ds))
	decoded, _ := base64.StdEncoding.DecodeString(ds.Payload)
	var st intoto.Statement
	assert.NoError(t, json.Unmarshal(decoded, &st))
	assert.Equal(t, "my-type", st.PredicateType)
	assert.Equal(t, "ssss", st.Subject[0].Digest.Sha256)
}

func TestAddSubjectAndStageToStatement_InvalidJSON(t *testing.T) {
	_, err := addSubjectAndStageToStatement([]byte("not-json"), "abc", "stage")
	assert.Error(t, err)
}

type failingArt struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (f *failingArt) FileInfo(_ string) (*utils.FileInfo, error) { return nil, errors.New("x") }

func TestBuildSonarStatement_FileInfoError(t *testing.T) {
	keyContent, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	c := &createEvidenceBase{stmtResolver: &fakeStmtResolver{out: []byte(`{"a":1}`)}, artifactoryClient: &failingArt{}, key: string(keyContent)}
	_, err := c.buildSonarStatement("r/p", "")
	assert.Error(t, err)
}

func TestBuildIntotoStatementJson_InvalidMarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	c := &createEvidenceBase{predicateFilePath: pred, predicateType: "t", markdownFilePath: filepath.Join(dir, "notes.txt"), serverDetails: &config.ServerDetails{User: "u"}}
	_, err := c.buildIntotoStatementJson("r/p", "")
	assert.Error(t, err)
}

func TestCreateAndSignEnvelope_KeyFromPath_Succeeds(t *testing.T) {
	// write ECDSA key to temp file and pass its path
	keyPath := filepath.Join(t.TempDir(), "key.pem")
	content, err := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(keyPath, content, 0600))
	env, err := createAndSignEnvelope([]byte(`{"p":true}`), keyPath, "kid")
	assert.NoError(t, err)
	assert.NotNil(t, env)
}

func pemEncodePKCS1RSAPrivateKey(key *rsa.PrivateKey) []byte {
	b := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: b})
}

func pemEncodePKCS8PrivateKey(key any) []byte {
	b, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})
}

func TestCreateAndSignEnvelope_RSAKey_Succeeds(t *testing.T) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	pemKey := pemEncodePKCS1RSAPrivateKey(k)
	env, err := createAndSignEnvelope([]byte(`{"a":1}`), string(pemKey), "kid")
	assert.NoError(t, err)
	assert.NotNil(t, env)
	assert.Equal(t, intoto.PayloadType, env.PayloadType)
	assert.True(t, len(env.Signatures) >= 1)
}

func TestCreateAndSignEnvelope_ED25519Key_Succeeds(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	assert.NoError(t, err)
	pemKey := pemEncodePKCS8PrivateKey(priv)
	env, err := createAndSignEnvelope([]byte(`{"a":1}`), string(pemKey), "kid2")
	assert.NoError(t, err)
	assert.NotNil(t, env)
	assert.Equal(t, intoto.PayloadType, env.PayloadType)
}

func TestCreateSigners_UnsupportedType_Error(t *testing.T) {
	_, err := createSigners(&cryptox.SSLibKey{KeyType: "unknown"})
	assert.Error(t, err)
}

func TestEnhanceKeyError_WithAndWithoutKeyId(t *testing.T) {
	errWith := enhanceKeyError(errors.New("boom"), "kid")
	assert.Error(t, errWith)
	assert.Contains(t, errWith.Error(), "key alias 'kid'")

	errWithout := enhanceKeyError(errors.New("boom"), "")
	assert.Error(t, errWithout)
	assert.Contains(t, errWithout.Error(), "failed to load private key")
}

func TestGetFileChecksum_Success(t *testing.T) {
	art := createFileInfoOnlyMock("abc")
	c := &createEvidenceBase{}
	sha, err := c.getFileChecksum("repo/p", art)
	assert.NoError(t, err)
	assert.Equal(t, "abc", sha)
}
