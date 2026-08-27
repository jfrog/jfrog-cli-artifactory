package npm

import (
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const (
	lockfileName       = "package-lock.json"
	shrinkwrapFileName = "npm-shrinkwrap.json"
)

func discoverProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, discoveryOptions{})
}

func discoverProjectRootWithOptions(workingDir string, opts discoveryOptions) (string, error) {
	startDir, err := effectiveStartDir(workingDir, opts)
	if err != nil {
		return "", err
	}
	dir := startDir
	var firstPackageJSON string
	var topWorkspaceRoot string
	for {
		pkgPath := filepath.Join(dir, "package.json")
		if _, statErr := os.Stat(pkgPath); statErr == nil {
			if firstPackageJSON == "" {
				firstPackageJSON = dir
			}
			if data, readErr := os.ReadFile(pkgPath); readErr == nil {
				if pkg, parseErr := parsePackageJSON(data); parseErr == nil && pkg.hasWorkspaces() {
					topWorkspaceRoot = dir
				}
			}
		}
		if _, statErr := os.Stat(filepath.Join(dir, shrinkwrapFileName)); statErr == nil {
			return dir, nil
		}
		if _, statErr := os.Stat(filepath.Join(dir, lockfileName)); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if topWorkspaceRoot != "" {
		return topWorkspaceRoot, nil
	}
	if firstPackageJSON != "" {
		return firstPackageJSON, nil
	}
	return "", errorutils.CheckErrorf("no %s or lockfile found from %s", "package.json", startDir)
}

func lockfileNameInDir(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, shrinkwrapFileName)); err == nil {
		return shrinkwrapFileName, nil
	}
	if _, err := os.Stat(filepath.Join(dir, lockfileName)); err == nil {
		return lockfileName, nil
	}
	return "", errorutils.CheckErrorf("no %s or %s under %s", shrinkwrapFileName, lockfileName, dir)
}
