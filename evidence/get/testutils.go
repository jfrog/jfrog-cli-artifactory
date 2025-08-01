package get

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReadTestDataFile reads a test data file from the shared testdata directory.
func ReadTestDataFile(filename string) (string, error) {
	testDataPath := filepath.Join("..", "testdata", "get", filename)
	data, err := os.ReadFile(testDataPath)
	if err != nil {
		return "", fmt.Errorf("failed to read test data file %s: %w", filename, err)
	}
	return string(data), nil
}
