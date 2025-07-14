package sigstore

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSubjectFromBundleV02(t *testing.T) {
	// Create test bundle JSON with v0.2 format
	statement := createTestStatementMap("test-repo/test-artifact", "abcd1234567890")
	payload := createTestPayload(t, statement)

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

	// Write to temp file
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	// Parse the bundle first
	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	// Test subject extraction from bundle
	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "test-repo/test-artifact", repoPath)
}

func TestExtractSubjectFromBundleV01(t *testing.T) {
	// Create test bundle JSON with v0.1 format (backward compatibility)
	statement := createTestStatementMap("legacy-repo/legacy-artifact", "1234567890abcd")
	payload := createTestPayload(t, statement)

	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.1",
		"verificationMaterial": {
			"x509CertificateChain": {
				"certificates": [
					{
						"rawBytes": "dGVzdC1jZXJ0"
					}
				]
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [
				{
					"sig": "bGVnYWN5LXNpZ25hdHVyZQ==",
					"keyid": ""
				}
			]
		}
	}`

	// Write to temp file
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-v01.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	// Parse the bundle
	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	// Test subject extraction
	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "legacy-repo/legacy-artifact", repoPath)
}

func TestExtractSubjectNoSubjects(t *testing.T) {
	// Create statement with no subjects
	statementNoSubjects := map[string]interface{}{
		"_type":         "https://in-toto.io/Statement/v1",
		"subject":       []interface{}{}, // Empty subjects
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statementNoSubjects)

	// Create a test bundle
	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdA=="
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [{"sig": "dGVzdA=="}]
		}
	}`

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-no-subjects.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)

	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "", repoPath) // Empty subjects should return empty repo path
}

func TestExtractSubjectNoSHA256(t *testing.T) {
	// Create statement with subject but no SHA256
	statementNoSHA := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"digest": map[string]interface{}{
					"sha512": "abcd", // Only SHA512, no SHA256
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statementNoSHA)

	// Create a test bundle
	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdA=="
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [{"sig": "dGVzdA=="}]
		}
	}`

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-no-sha256.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)

	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "", repoPath) // Missing SHA256 should still return empty repo path since no name field
}

func TestExtractSubjectNoRepoPath(t *testing.T) {
	// Create statement without repo path in subjects
	statementNoPath := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"digest": map[string]interface{}{
					"sha256": "abcd1234",
				},
				// No name field
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     map[string]interface{}{},
	}

	payload := createTestPayload(t, statementNoPath)

	// Create a test bundle
	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdA=="
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [{"sig": "dGVzdA=="}]
		}
	}`

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-no-path.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)

	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "", repoPath) // Empty repo path is allowed
}

func TestExtractSubjectWithGitHubPredicate(t *testing.T) {
	// Test with a GitHub-specific predicate that might have subject in different location
	statement := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"name": "pkg:github/example/artifact@v1.0.0",
				"digest": map[string]interface{}{
					"sha256": "fedcba9876543210",
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v1.0",
		"predicate": map[string]interface{}{
			"buildDefinition": map[string]interface{}{
				"resolvedDependencies": []interface{}{
					map[string]interface{}{
						"uri":    "git+https://github.com/example/repo@refs/heads/main",
						"digest": map[string]interface{}{"sha1": "abc123"},
					},
				},
			},
			"runDetails": map[string]interface{}{
				"builder": map[string]interface{}{
					"id": "https://github.com/actions/runner/v2.311.0",
				},
			},
		},
	}

	payload := createTestPayload(t, statement)

	// Create a test bundle
	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdA=="
			}
		},
		"dsseEnvelope": {
			"payload": "` + payload + `",
			"payloadType": "application/vnd.in-toto+json",
			"signatures": [{"sig": "dGVzdA=="}]
		}
	}`

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "test-bundle-github.json")
	err := os.WriteFile(bundlePath, []byte(bundleJSON), 0644)
	assert.NoError(t, err)

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)

	repoPath, err := ExtractSubjectFromBundle(bundle)
	assert.NoError(t, err)
	assert.Equal(t, "pkg:github/example/artifact@v1.0.0", repoPath)
}

func createTestStatementMap(repoPath, sha256 string) map[string]interface{} {
	// Use predicate with artifact path if repoPath is provided
	predicate := map[string]interface{}{
		"builder": map[string]interface{}{
			"id": "https://github.com/actions/runner/v2.311.0",
		},
		"buildType": "https://github.com/actions/runner/v2.311.0",
		"invocation": map[string]interface{}{
			"configSource": map[string]interface{}{
				"uri":        "https://github.com/example/repo",
				"digest":     map[string]interface{}{"sha1": "abcdef123456"},
				"entryPoint": ".github/workflows/build.yaml",
			},
		},
	}

	if repoPath != "" {
		predicate["artifact"] = map[string]interface{}{
			"path": repoPath,
		}
	}

	return map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []interface{}{
			map[string]interface{}{
				"digest": map[string]interface{}{
					"sha256": sha256,
				},
			},
		},
		"predicateType": "https://slsa.dev/provenance/v0.2",
		"predicate":     predicate,
	}
}

func createTestPayload(t *testing.T, statement interface{}) string {
	statementBytes, err := json.Marshal(statement)
	assert.NoError(t, err)
	return base64.StdEncoding.EncodeToString(statementBytes)
}
