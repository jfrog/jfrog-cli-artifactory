package sigstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBundleRealFile(t *testing.T) {
	bundlePath := filepath.Join("testdata", "sample-bundle.json")

	bundle, err := ParseBundle(bundlePath)
	assert.NoError(t, err)
	assert.NotNil(t, bundle)

	envelope, err := GetDSSEEnvelope(bundle)
	assert.NoError(t, err)
	assert.NotNil(t, envelope)
	assert.Equal(t, "application/vnd.in-toto+json", envelope.PayloadType)
	assert.Len(t, envelope.Signatures, 1)
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
