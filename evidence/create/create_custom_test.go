package create

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/sigstore"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/create/resolvers"
	"github.com/jfrog/jfrog-client-go/artifactory"
	artutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	evdservices "github.com/jfrog/jfrog-client-go/evidence/services"
)

// subjectLookupFunc is a test adapter implementing resolvers.SubjectLookup
type subjectLookupFunc func(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error)

func (f subjectLookupFunc) ResolveSubject(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error) {
	return f(subject, checksum, client)
}

type failingUploaderCustom struct{ err error }

func (f *failingUploaderCustom) UploadEvidence(e evdservices.EvidenceDetails) ([]byte, error) {
	return nil, f.err
}

type mockArtifactoryServicesManagerCustom struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManagerCustom) FileInfo(_ string) (*artutils.FileInfo, error) {
	return &artutils.FileInfo{Checksums: struct {
		Sha1   string `json:"sha1,omitempty"`
		Sha256 string `json:"sha256,omitempty"`
		Md5    string `json:"md5,omitempty"`
	}{Sha256: "sha"}}, nil
}

type captureUploaderCustom struct{ last evdservices.EvidenceDetails }

func (c *captureUploaderCustom) UploadEvidence(d evdservices.EvidenceDetails) ([]byte, error) {
	resp := model.CreateResponse{PredicateSlug: "slug", Verified: true, PredicateType: "ptype"}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	c.last = d
	return b, nil
}

func TestCreateEvidenceCustom_Run_DSSE_Success(t *testing.T) {
	// Prepare predicate and key
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))

	// Inject artifactory and uploader
	art := &mockArtifactoryServicesManagerCustom{}
	upl := &captureUploaderCustom{}
	c := &createEvidenceCustom{
		createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{User: "u"}, predicateFilePath: pred, predicateType: "ptype", key: string(key), artifactoryClient: art, uploader: upl},
		subjectRepoPaths:   []string{"repo/path/name"},
		subjectSha256:      "",
	}
	err := c.Run()
	assert.NoError(t, err)
	assert.NotNil(t, upl.last.DSSEFileRaw)
	assert.Equal(t, "repo/path/name", upl.last.SubjectUri)
}

func TestExtractSubjectFromBundle_ResolverErrorMapped(t *testing.T) {
	// Build a bundle with subject and sha256
	statement := map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{
			map[string]any{
				"digest": map[string]any{"sha256": "abc"},
				"name":   "repo/path:tag",
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]any{},
	}
	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)
	bundleJSON := `{"mediaType":"application/vnd.dev.sigstore.bundle+json;version=0.2","verificationMaterial":{"certificate":{"rawBytes":"ZGF0YQ=="}},"dsseEnvelope":{"payload":"` + payload + `","payloadType":"application/vnd.in-toto+json","signatures":[{"sig":"ZGF0YQ==","keyid":"id"}]}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.json")
	assert.NoError(t, os.WriteFile(path, []byte(bundleJSON), 0600))

	b, err := sigstore.ParseBundle(path)
	assert.NoError(t, err)

	// Inject a non-nil artifactory client and a lookup that returns error
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{Url: "http://example.com"}, artifactoryClient: &mockArtifactoryServicesManagerCustom{}}}
	c.lookup = subjectLookupFunc(func(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error) {
		return nil, errorutils.CheckErrorf("boom")
	})
	_, err = c.extractSubjectFromBundle(b)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve subject")
}

func TestCreateEvidenceCustom_Run_DSSE_SigningError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	// Use public key to cause signing error
	pub, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/public_key.pem"))
	art := &mockArtifactoryServicesManagerCustom{}
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(pub), artifactoryClient: art}, subjectRepoPaths: []string{"repo/a"}}
	err := c.Run()
	assert.Error(t, err)
}

type failingUploaderCustom404 struct{}

func (f *failingUploaderCustom404) UploadEvidence(e evdservices.EvidenceDetails) ([]byte, error) {
	return nil, errorutils.CheckErrorf("server response: 404 Not Found")
}

func TestCreateEvidenceCustom_Run_Upload404Mapped_ManualMode(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := &mockArtifactoryServicesManagerCustom{}
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art, uploader: &failingUploaderCustom404{}}, subjectRepoPaths: []string{"repo/a"}}
	err := c.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not found")
}

func TestProcessSigstoreBundle_AutoResolutionAndSubjectsSet(t *testing.T) {
	// Build a valid bundle with subject name and sha
	statement := map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{
			map[string]any{
				"digest": map[string]any{"sha256": "abc"},
				"name":   "repo/path/artifact",
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]any{},
	}
	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)
	bundleJSON := `{
        "mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
        "verificationMaterial": {"certificate": {"rawBytes": "dGVzdC1jZXJ0"}},
        "dsseEnvelope": {"payload": "` + payload + `", "payloadType": "application/vnd.in-toto+json", "signatures": [{"sig": "dGVzdC1zaWduYXR1cmU=", "keyid": "id"}]}
    }`
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.json")
	assert.NoError(t, os.WriteFile(path, []byte(bundleJSON), 0600))

	art := &mockArtifactoryServicesManagerCustom{}
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{artifactoryClient: art}, sigstoreBundlePath: path}
	c.lookup = resolvers.DefaultSubjectLookup{}
	out, err := c.processSigstoreBundle()
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.True(t, c.autoSubjectResolution)
	assert.Equal(t, []string{"repo/path/artifact"}, c.subjectRepoPaths)
}

func TestExtractSubjectFromBundle_ResolverReturnsEmpty_NewSubjectError(t *testing.T) {
	// Build bundle with subject
	statement := map[string]any{"_type": "https://in-toto.io/Statement/v1", "subject": []any{map[string]any{"digest": map[string]any{"sha256": "abc"}, "name": "repo/path/artifact"}}, "predicateType": "https://slsa.dev/provenance/v0.2", "predicate": map[string]any{}}
	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)
	bundleJSON := `{"mediaType":"application/vnd.dev.sigstore.bundle+json;version=0.2","verificationMaterial":{"certificate":{"rawBytes":"ZGF0YQ=="}},"dsseEnvelope":{"payload":"` + payload + `","payloadType":"application/vnd.in-toto+json","signatures":[{"sig":"ZGF0YQ==","keyid":"id"}]}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.json")
	assert.NoError(t, os.WriteFile(path, []byte(bundleJSON), 0600))

	b, err := sigstore.ParseBundle(path)
	assert.NoError(t, err)

	c := &createEvidenceCustom{}
	// Inject resolver that returns empty slice
	c.lookup = subjectLookupFunc(func(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error) {
		return []string{}, nil
	})
	// Ensure artifactory client creation has valid server details
	c.serverDetails = &config.ServerDetails{Url: "http://example.com"}
	_, err = c.extractSubjectFromBundle(b)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Subject resolution returned no results")
}

func TestCreateEvidenceCustom_Run_ValidationError(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := &mockArtifactoryServicesManagerCustom{}
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art}, subjectRepoPaths: []string{"invalid"}}
	err := c.Run()
	assert.Error(t, err)
}

func TestCreateEvidenceCustom_Run_SubjectsLimitExceeded(t *testing.T) {
	dir := t.TempDir()
	pred := filepath.Join(dir, "p.json")
	assert.NoError(t, os.WriteFile(pred, []byte(`{"a":1}`), 0600))
	key, _ := os.ReadFile(filepath.Join("../..", "tests/testdata/ecdsa_key.pem"))
	art := &mockArtifactoryServicesManagerCustom{}
	paths := make([]string, subjectsLimit+1)
	for i := range paths {
		paths[i] = "repo/a"
	}
	c := &createEvidenceCustom{createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: pred, predicateType: "t", key: string(key), artifactoryClient: art}, subjectRepoPaths: paths}
	err := c.Run()
	assert.Error(t, err)
}

func TestNewCreateEvidenceCustom(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	// Test with regular evidence creation (no sigstore bundle)
	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"predicate.json",
		"https://example.com/predicate/v1",
		"markdown.md",
		"key.pem",
		"key-alias",
		"test-repo/test-artifact",
		"abcd1234",
		"", // No sigstore bundle
		"test-integration",
		"",
	)

	assert.NotNil(t, cmd)
	assert.Equal(t, "create-custom-evidence", cmd.CommandName())
	details, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, details)
}

func TestCreateEvidenceCustom_WithSigstoreBundle(t *testing.T) {
	// Create a test bundle file using generic map
	statement := map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{
			map[string]any{
				"digest": map[string]any{
					"sha256": "test-sha256",
				},
				"name": "test-repo/test-artifact",
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate": map[string]any{
			"builder": map[string]any{
				"id": "https://github.com/actions/runner/v2.311.0",
			},
			"artifact": map[string]any{
				"path": "test-repo/test-artifact",
			},
		},
	}

	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)

	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdC1jZXJ0"
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [
				{
					"sig": "dGVzdC1zaWduYXR1cmU=",
					"keyid": "test-key-id"
				}
			]
		}
	}`

	// Write bundle to temp file
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle.json")
	err = os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	// Create command with sigstore bundle
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}
	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"", // No predicate file
		"", // No predicate type
		"", // No markdown
		"", // No key
		"", // No key alias
		"",
		"",         // No sha256 (will be extracted from bundle)
		bundlePath, // Sigstore bundle path
		"test-integration",
		"",
	)

	// Verify command setup
	assert.NotNil(t, cmd)
	assert.Equal(t, "create-custom-evidence", cmd.CommandName())
}

func TestCreateEvidenceCustom_MissingSigstoreBundle(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	// Create command with non-existent bundle file
	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"", // No predicate file
		"", // No predicate type
		"", // No markdown
		"", // No key
		"", // No key alias
		"test-repo/test-artifact",
		"",
		"/non/existent/bundle.json", // Non-existent bundle
		"test-integration",
		"",
	)

	// Run should fail
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read sigstore bundle")
}

func TestCreateEvidenceCustom_UploadError(t *testing.T) {
	// Create a test predicate file
	predicateContent := `{"key": "value"}`
	tmpDir := t.TempDir()
	predicatePath := filepath.Join(tmpDir, "predicate.json")
	err := os.WriteFile(predicatePath, []byte(predicateContent), 0644)
	assert.NoError(t, err)

	// Set up a temporary directory for the summary
	summaryDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, fileutils.RemoveTempDir(summaryDir))
	}()
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, summaryDir))
	defer func() {
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()

	// Build command instance directly to inject failing uploader
	custom := &createEvidenceCustom{
		createEvidenceBase: createEvidenceBase{serverDetails: &config.ServerDetails{}, predicateFilePath: predicatePath, predicateType: "custom-predicate", uploader: &failingUploaderCustom{err: assert.AnError}},
		subjectRepoPaths:   []string{"test-repo/test-artifact"},
		subjectSha256:      "sha256:12345",
	}
	runErr := custom.Run()
	assert.Error(t, runErr)

	summaryFiles, err := fileutils.ListFiles(summaryDir, true)
	assert.NoError(t, err)
	assert.Empty(t, summaryFiles)
}

func TestCreateEvidenceCustom_SigstoreBundleWithSubjectPath(t *testing.T) {
	// Create a test bundle without artifact path in predicate
	statement := map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{
			map[string]any{
				"digest": map[string]any{
					"sha256": "extracted-sha256",
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]any{},
	}

	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)

	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdC1jZXJ0"
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [
				{
					"sig": "dGVzdC1zaWduYXR1cmU=",
					"keyid": "test-key-id"
				}
			]
		}
	}`

	// Write bundle to temp file
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-no-path.json")
	err = os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	// Create command with explicit subject path (since bundle doesn't have it)
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}
	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"",                                // No predicate file
		"",                                // No predicate type
		"",                                // No markdown
		"",                                // No key
		"",                                // No key alias
		"provided-repo/provided-artifact", // This should be used as fallback
		"",
		bundlePath,
		"test-integration",
		"",
	)

	// Verify the command would use the provided subject path
	assert.NotNil(t, cmd)
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok, "cmd should be of type *createEvidenceCustom")
	assert.Equal(t, bundlePath, custom.sigstoreBundlePath)
	assert.Equal(t, []string{"provided-repo/provided-artifact"}, custom.subjectRepoPaths)
}

func TestCreateEvidenceCustom_NewSubjectError_AutoSubjectResolution(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"predicate.json",
		"https://example.com/predicate/v1",
		"markdown.md",
		"key.pem",
		"key-alias",
		"",
		"abcd1234",
		"/path/to/sigstore-bundle.json",
		"test-integration",
		"",
	)

	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok, "cmd should be of type *createEvidenceCustom")

	custom.autoSubjectResolution = true

	testMessage := "Test error message"
	err := custom.newSubjectError(testMessage)

	assert.Error(t, err)
	cliErr, ok := err.(coreutils.CliError)
	assert.True(t, ok, "error should be of type CliError when autoSubjectResolution is enabled")
	assert.Equal(t, coreutils.ExitCodeFailNoOp, cliErr.ExitCode, "should return exit code 2 (ExitCodeFailNoOp)")
	assert.Equal(t, testMessage, cliErr.ErrorMsg, "error message should match")
}

func TestCreateEvidenceCustom_NewSubjectError_RegularExecution(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(
		serverDetails,
		"predicate.json",
		"https://example.com/predicate/v1",
		"markdown.md",
		"key.pem",
		"key-alias",
		"test-repo/test-artifact",
		"abcd1234",
		"",
		"test-integration",
		"",
	)

	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok, "cmd should be of type *createEvidenceCustom")

	custom.autoSubjectResolution = false

	testMessage := "Test error message"
	err := custom.newSubjectError(testMessage)

	assert.Error(t, err)
	_, ok = err.(coreutils.CliError)
	assert.False(t, ok, "error should not be of type CliError when autoSubjectResolution is disabled")
	assert.Contains(t, err.Error(), testMessage, "error message should contain the test message")
}

func TestCreateEvidenceCustom_RecordSummary(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, fileutils.RemoveTempDir(tempDir))
	}()

	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()

	serverDetails := &config.ServerDetails{
		Url:      "http://test.com",
		User:     "testuser",
		Password: "testpass",
	}

	subjectRepoPath := "test-repo/path/to/artifact.jar"
	subjectSha256 := "custom-sha256"

	evidence := NewCreateEvidenceCustom(
		serverDetails,
		"",
		"custom-predicate-type",
		"",
		"test-key",
		"test-key-id",
		subjectRepoPath,
		subjectSha256,
		"",
		"test-integration",
		"",
	)
	c, ok := evidence.(*createEvidenceCustom)
	assert.True(t, ok, "should create createEvidenceCustom instance")

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "custom-slug",
		Verified:      false,
		PredicateType: "custom-predicate-type",
	}

	c.recordSummary(subjectRepoPath, expectedResponse)

	summaryFiles, err := fileutils.ListFiles(tempDir, true)
	assert.NoError(t, err)
	assert.True(t, len(summaryFiles) > 0, "Summary file should be created")

	for _, file := range summaryFiles {
		if strings.HasSuffix(file, "-data") {
			content, err := os.ReadFile(file)
			assert.NoError(t, err)

			var summaryData commandsummary.EvidenceSummaryData
			err = json.Unmarshal(content, &summaryData)
			assert.NoError(t, err)

			assert.Equal(t, subjectRepoPath, summaryData.Subject)
			assert.Equal(t, subjectSha256, summaryData.SubjectSha256)
			assert.Equal(t, "custom-predicate-type", summaryData.PredicateType)
			assert.Equal(t, "custom-slug", summaryData.PredicateSlug)
			assert.False(t, summaryData.Verified)
			assert.Equal(t, subjectRepoPath, summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypeArtifact, summaryData.SubjectType)
			assert.Equal(t, subjectRepoPath, summaryData.RepoKey)
			break
		}
	}
}

func TestCreateEvidenceCustom_DetermineFinalError_AutoResolution_AllSuccess(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	custom.autoSubjectResolution = true

	// Test with no errors (all subjects succeeded)
	err := custom.determineFinalError([]error{}, []string{"repo1/artifact1", "repo2/artifact2"}, []string{})
	assert.NoError(t, err)
}

func TestCreateEvidenceCustom_DetermineFinalError_AutoResolution_PartialSuccess(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	custom.autoSubjectResolution = true

	// Test with partial success (some subjects succeeded, some failed)
	errors := []error{
		errorutils.CheckErrorf("validation failed for subject 'repo2/invalid': invalid format"),
		errorutils.CheckErrorf("upload failed for subject 'repo3/missing': 404 Not Found"),
	}
	successfulSubjects := []string{"repo1/artifact1", "repo4/artifact4"}
	failedSubjects := []string{"repo2/invalid", "repo3/missing"}

	err := custom.determineFinalError(errors, successfulSubjects, failedSubjects)
	assert.Error(t, err)

	cliErr, ok := err.(coreutils.CliError)
	assert.True(t, ok, "error should be of type CliError for auto-resolution with partial success")
	assert.Equal(t, coreutils.ExitCodeFailNoOp, cliErr.ExitCode)
	assert.Contains(t, cliErr.ErrorMsg, "Partially successful: 2 succeeded, 2 failed")
	assert.Contains(t, cliErr.ErrorMsg, "repo2/invalid")
	assert.Contains(t, cliErr.ErrorMsg, "repo3/missing")
}

func TestCreateEvidenceCustom_DetermineFinalError_AutoResolution_AllFailed(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	custom.autoSubjectResolution = true

	// Test with all subjects failed
	errors := []error{
		errorutils.CheckErrorf("validation failed for subject 'repo1/invalid': invalid format"),
		errorutils.CheckErrorf("upload failed for subject 'repo2/missing': 404 Not Found"),
	}
	successfulSubjects := []string{}
	failedSubjects := []string{"repo1/invalid", "repo2/missing"}

	err := custom.determineFinalError(errors, successfulSubjects, failedSubjects)
	assert.Error(t, err)

	cliErr, ok := err.(coreutils.CliError)
	assert.True(t, ok, "error should be of type CliError for auto-resolution with all failed")
	assert.Equal(t, coreutils.ExitCodeFailNoOp, cliErr.ExitCode)
	assert.Contains(t, cliErr.ErrorMsg, "Failed to process 2 subjects")
	assert.Contains(t, cliErr.ErrorMsg, "repo1/invalid")
	assert.Contains(t, cliErr.ErrorMsg, "repo2/missing")
}

func TestCreateEvidenceCustom_DetermineFinalError_ManualMode_Success(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	custom.autoSubjectResolution = false

	// Test manual mode with no errors (single subject succeeded)
	err := custom.determineFinalError([]error{}, []string{"repo1/artifact1"}, []string{})
	assert.NoError(t, err)
}

func TestCreateEvidenceCustom_DetermineFinalError_ManualMode_Failure(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	custom.autoSubjectResolution = false

	// Test manual mode with error (single subject failed)
	expectedError := errorutils.CheckErrorf("validation failed for subject 'repo1/invalid': invalid format")
	errors := []error{expectedError}
	successfulSubjects := []string{}
	failedSubjects := []string{"repo1/invalid"}

	err := custom.determineFinalError(errors, successfulSubjects, failedSubjects)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err, "manual mode should return the original error directly")
}

func TestCreateEvidenceCustom_ExtractSubjectFromBundle_ImprovedErrorHandling(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	// Test with bundle that has empty subject name (missing "name" field)
	statement := map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{
			map[string]any{
				"digest": map[string]any{
					"sha256": "test-sha256",
				},
				// Missing "name" field - this should cause extraction to return empty subject
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]any{},
	}

	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	payload := base64.StdEncoding.EncodeToString(statementBytes)

	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdC1jZXJ0"
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [
				{
					"sig": "dGVzdC1zaWduYXR1cmU=",
					"keyid": "test-key-id"
				}
			]
		}
	}`

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-empty-subject.json")
	err = os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	// Parse the bundle to get a valid bundle object
	bundle, err := sigstore.ParseBundle(bundlePath)
	assert.NoError(t, err)

	// Test that the improved error handling provides better context
	// This should fail because the subject has no "name" field, resulting in empty subject
	_, err = custom.extractSubjectFromBundle(bundle)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract subject from bundle: name was not found in DSSE subject")
}

func TestCreateEvidenceCustom_ValidateSubject_ImprovedErrorContext(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	// Test valid subject
	err := custom.validateSubject("repo/artifact")
	assert.NoError(t, err)

	// Test invalid subject
	err = custom.validateSubject("invalid-subject")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Subject 'invalid-subject' is invalid")
	assert.Contains(t, err.Error(), "format: <repo>/<path>/<name> or <repo>/<name>")

	// Test empty subject
	err = custom.validateSubject("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Subject '' is invalid")
}

func TestCreateEvidenceCustom_HandleSubjectNotFound_ImprovedErrorContext(t *testing.T) {
	serverDetails := &config.ServerDetails{
		Url:         "https://test.jfrog.io",
		User:        "test-user",
		AccessToken: "test-token",
	}

	cmd := NewCreateEvidenceCustom(serverDetails, "", "", "", "", "", "", "", "", "", "")
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok)

	// Test 404 error
	originalErr := errorutils.CheckErrorf("404 Not Found")
	subjectPath := "repo/missing-artifact"
	err := custom.handleSubjectNotFound(subjectPath, originalErr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Subject 'repo/missing-artifact' is not found")

	// Test non-404 error (should pass through unchanged)
	otherErr := errorutils.CheckErrorf("500 Internal Server Error")
	err = custom.handleSubjectNotFound(subjectPath, otherErr)
	assert.Error(t, err)
	assert.Equal(t, otherErr, err)
}
