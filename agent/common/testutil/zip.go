package testutil

import (
	"archive/zip"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTestZip builds a zip file at zipPath containing the given file paths and contents.
func CreateTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	w := zip.NewWriter(f)
	defer func() {
		_ = w.Close()
	}()

	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}
