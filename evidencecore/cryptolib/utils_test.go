package cryptolib

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateKeyID(t *testing.T) {
	key := &SSLibKey{
		KeyIDHashAlgorithms: nil,
		KeyType:             "rsa",
		KeyVal:              KeyVal{},
		Scheme:              "",
		KeyID:               "",
	}
	keyID, err := calculateKeyID(key)
	assert.NoError(t, err)
	// Check if the returned key ID matches the expected one
	// #nosec G101 - False positive - Not a real password
	expectedKeyID := "f97abd1db1e58debee59bf72ce05a31c77f58df54e3ff47eb532270e37f2f12b" // replace with the expected key ID
	if keyID != expectedKeyID {
		t.Errorf("Expected '%s', got '%s'", expectedKeyID, keyID)
	}
}

func TestGeneratePEMBlock(t *testing.T) {
	pem := generatePEMBlock([]byte("key"), "pemType")
	assert.Equal(t, "-----BEGIN pemType-----\na2V5\n-----END pemType-----\n", string(pem))
}

func TestDecodeParsePEM(t *testing.T) {
	pem, _, err := decodeAndParsePEM(rsaPrivateKey)
	assert.NoError(t, err)
	assert.Equal(t, "RSA PRIVATE KEY", pem.Type)
}

func TestParsePEMKey(t *testing.T) {
	pem, _, err := decodeAndParsePEM(rsaPrivateKey)
	assert.NoError(t, err)
	key, err := parsePEMKey(pem.Bytes)
	assert.NoError(t, err)
	assert.NotNil(t, key)
}

func TestHashBeforeSigning(t *testing.T) {
	// Call hashBeforeSigning with a known payload and a SHA256 hash function
	payload := "test payload"
	hash := hashBeforeSigning([]byte(payload), sha256.New())

	// Check if the returned hash matches the expected one
	// #nosec G101 - False positive - Not a real password
	expectedHash := "813ca5285c28ccee5cab8b10ebda9c908fd6d78ed9dc94cc65ea6cb67a7f13ae" // SHA256 hash of "test payload"
	if hex.EncodeToString(hash) != expectedHash {
		t.Errorf("Expected '%s', got '%s'", expectedHash, hex.EncodeToString(hash))
	}
}
