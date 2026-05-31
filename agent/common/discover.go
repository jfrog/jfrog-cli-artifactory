package common

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DiscoverInstalledSlugs returns the sorted slugs installed under installDir whose
// install directories contain a .jfrog/<manifestFileName>. A missing installDir
// returns an empty slice with no error so callers can treat "no harness install root yet"
// as "nothing installed".
func DiscoverInstalledSlugs(installDir, manifestFileName string) ([]string, error) {
	entries, err := os.ReadDir(installDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read install dir %s: %w", installDir, err)
	}
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(installDir, entry.Name(), jfrogInstallDirName, manifestFileName)
		info, err := os.Stat(manifestPath)
		if err != nil || info.IsDir() {
			continue
		}
		slugs = append(slugs, entry.Name())
	}
	sort.Strings(slugs)
	return slugs, nil
}
