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
//
// Discovery checks each immediate child directory of installDir for a regular file at
// <installDir>/<slug>/.jfrog/<manifestFileName> (see installInfoManifestPath).
//
// Example (manifestFileName = "plugin-info.json", installDir = ".claude/plugins"):
//
//	.claude/plugins/
//	  web-search/                    -> discovered (slug "web-search")
//	    .jfrog/plugin-info.json
//	  legacy/                        -> skipped (no .jfrog/plugin-info.json)
//	    plugin.json
//
// Returns []string{"web-search"}.
func DiscoverInstalledSlugs(installDir, manifestFileName string) ([]string, error) {
	// ReadDir may return a non-nil error together with entries read before the failure; scan those too.
	entries, readErr := os.ReadDir(installDir)
	if readErr != nil && errors.Is(readErr, os.ErrNotExist) {
		return nil, nil
	}
	slugs := slugsFromInstallDirEntries(installDir, manifestFileName, entries)
	if readErr != nil {
		return slugs, fmt.Errorf("read install dir %s: %w", installDir, readErr)
	}
	return slugs, nil
}

func slugsFromInstallDirEntries(installDir, manifestFileName string, entries []os.DirEntry) []string {
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
	return slugs
}
