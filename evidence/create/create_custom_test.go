package create

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

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
		"test-provider",
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
		"test-provider",
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
		"test-provider",
	)

	// Run should fail
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read sigstore bundle")
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
		"test-provider",
	)

	// Verify the command would use the provided subject path
	assert.NotNil(t, cmd)
	custom, ok := cmd.(*createEvidenceCustom)
	assert.True(t, ok, "cmd should be of type *createEvidenceCustom")
	assert.Equal(t, bundlePath, custom.sigstoreBundlePath)
	assert.Equal(t, "provided-repo/provided-artifact", custom.subjectRepoPath)
}
