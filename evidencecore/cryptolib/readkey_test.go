package cryptolib

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadKey(t *testing.T) {
	files, err := os.ReadDir("test-data")
	assert.Nil(t, err)
	assert.Equal(t, 14, len(files))
	var keyFiles []os.DirEntry
	keysToValidate := []string{"ecdsa-test-key-pem", "ecdsa-test-key-pem.pub", "ed25519-test-key-pem", "ed25519-test-key-pem.pub", "rsa-test-key", "rsa-test-key.pub"}
	for _, file := range files {
		for _, key := range keysToValidate {
			if file.Name() == key {
				keyFiles = append(keyFiles, file)
			}
		}

	}
	keys, err := ReadKey(keyFiles, "test-data")
	assert.Nil(t, err)
	assert.NotNil(t, keys)
	assert.Equal(t, 6, len(keyFiles))

}
