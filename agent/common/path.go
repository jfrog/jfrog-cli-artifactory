package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateExistingDir requires path to exist and be a directory (after filepath.Abs).
func ValidateExistingDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	return nil
}

// ExpandHome maps a leading "~/" to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
