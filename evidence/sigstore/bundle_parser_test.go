package sigstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBundleV02(t *testing.T) {
	// Create test bundle JSON with v0.2 format
	bundleJSON := `{
		"mediaType": "application/vnd.dev.sigstore.bundle+json;version=0.2",
		"verificationMaterial": {
			"certificate": {
				"rawBytes": "dGVzdC1jZXJ0"
			}
		},
		"dsseEnvelope": {
			"payload": "dGVzdCBwYXlsb2Fk",
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

	// Test parsing
	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	// Test DSSE extraction
	envelope, err := GetDSSEEnvelope(bundle)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
	assert.Equal(t, "application/vnd.in-toto+json", envelope.PayloadType)
	assert.Len(t, envelope.Signatures, 1)
}

func TestParseBundleV01(t *testing.T) {
	// Create test bundle JSON with v0.1 format (backward compatibility)
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
			"payload": "dGVzdCBwYXlsb2Fk",
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

	// Test parsing
	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	// Test DSSE extraction
	envelope, err := GetDSSEEnvelope(bundle)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
}

func TestParseBundleInvalidFile(t *testing.T) {
	_, err := ParseBundle("/non/existent/file.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse sigstore bundle")
}

func TestParseBundleInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(bundlePath, []byte("invalid json"), 0644)
	assert.NoError(t, err)

	_, err = ParseBundle(bundlePath)
	assert.Error(t, err)
}
