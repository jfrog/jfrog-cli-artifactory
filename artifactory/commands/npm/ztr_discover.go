package npm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const (
	lockfileName       = "package-lock.json"
	shrinkwrapFileName = "npm-shrinkwrap.json"
)

type discoveryOptions struct {
	prefixDir string
}

func discoverProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, discoveryOptions{})
}

func effectiveStartDir(workingDir string, opts discoveryOptions) (string, error) {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if opts.prefixDir != "" {
		return resolveDiscoveryPath(abs, opts.prefixDir)
	}
	return abs, nil
}

// resolveDiscoveryPath joins base and p unless p is already absolute.
// On Windows, Unix-style paths are not filepath.IsAbs but must not be joined with base.
func resolveDiscoveryPath(base, p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	if strings.HasPrefix(filepath.ToSlash(p), "/") {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", errorutils.CheckError(err)
		}
		return filepath.Clean(abs), nil
	}
	return filepath.Clean(filepath.Join(base, p)), nil
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
	for _, name := range []string{shrinkwrapFileName, lockfileName} {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", errorutils.CheckError(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		return name, nil
	}
	return "", errorutils.CheckErrorf("no %s or %s under %s", shrinkwrapFileName, lockfileName, dir)
}

type packageJSON struct {
	Workspaces json.RawMessage `json:"workspaces"`
}

func parsePackageJSON(data []byte) (packageJSON, error) {
	var p packageJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return packageJSON{}, err
	}
	return p, nil
}

func (p packageJSON) hasWorkspaces() bool {
	if len(p.Workspaces) == 0 || string(p.Workspaces) == "null" {
		return false
	}
	if string(p.Workspaces) == "[]" {
		return false
	}
	var arr []any
	if json.Unmarshal(p.Workspaces, &arr) == nil {
		return len(arr) > 0
	}
	var obj struct {
		Packages []any `json:"packages"`
	}
	if json.Unmarshal(p.Workspaces, &obj) == nil {
		return len(obj.Packages) > 0
	}
	return true
}
