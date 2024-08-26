package cryptox

import (
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestReadKey(t *testing.T) {
	files, err := os.ReadDir("testdata")
	assert.NoError(t, err)
	assert.Equal(t, 20, len(files))
	var keyFiles []os.DirEntry
	keysToValidate := []string{"ecdsa-test-key-pem", "ed25519-test-key-pem", "rsa-test-key"}
	for _, file := range files {
		for _, key := range keysToValidate {
			if file.Name() == key {
				keyFiles = append(keyFiles, file)
			}
		}

	}
	assert.Equal(t, 3, len(keyFiles))

	for _, file := range keyFiles {
		keyFile, err := os.ReadFile(filepath.Join("testdata", file.Name()))
		assert.Nil(t, err)
		keys, err := ReadKey(keyFile)
		assert.Nil(t, err)
		assert.NotNil(t, keys)
	}
}
