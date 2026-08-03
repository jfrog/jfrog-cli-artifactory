package apmcommon

import (
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"gopkg.in/yaml.v3"
)

const ApmLockfileName = "apm.lock.yaml"

// ApmLockFile represents apm.lock.yaml. The real schema is a flat list under "dependencies" —
// confirmed live against a real `apm install` run — not a map under "packages".
type ApmLockFile struct {
	LockfileVersion string             `yaml:"lockfile_version"`
	Dependencies    []ApmLockedPackage `yaml:"dependencies"`
}

type ApmLockedPackage struct {
	RepoURL      string `yaml:"repo_url"` // "owner/repo"
	Name         string `yaml:"name"`
	Version      string `yaml:"version"`
	PackageType  string `yaml:"package_type"`
	Source       string `yaml:"source"` // "registry" for Artifactory-resolved deps
	ContentHash  string `yaml:"content_hash"`
	ResolvedURL  string `yaml:"resolved_url"` // full agentpackages download URL
	ResolvedHash string `yaml:"resolved_hash"`
}

// LoadLockFile reads and parses apm.lock.yaml at path. Every caller constructs path from a
// directory (the working directory, or install's own --root value) joined with the fixed
// ApmLockfileName ("apm.lock.yaml") - never a raw, arbitrary user-supplied filename.
func LoadLockFile(path string) (*ApmLockFile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always <dir>/apm.lock.yaml; the same apm invocation already wrote to this exact directory itself when --root was passed
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	var lockfile ApmLockFile
	if err = yaml.Unmarshal(data, &lockfile); err != nil {
		return nil, errorutils.CheckErrorf("parsing %s: %s", ApmLockfileName, err.Error())
	}
	return &lockfile, nil
}

// RegistryPackages returns only dependencies with source=registry.
func (l *ApmLockFile) RegistryPackages() []ApmLockedPackage {
	var result []ApmLockedPackage
	for _, pkg := range l.Dependencies {
		if pkg.Source == "registry" {
			result = append(result, pkg)
		}
	}
	return result
}

// DepID returns the build-info dependency ID: "owner/repo:version".
func (pkg ApmLockedPackage) DepID() string {
	return pkg.RepoURL + ":" + pkg.Version
}

// SHA256Hex extracts the hex-encoded SHA-256 from a "sha256:<hex>" string.
func SHA256Hex(resolvedHash string) string {
	const prefix = "sha256:"
	if !strings.HasPrefix(resolvedHash, prefix) {
		return ""
	}
	return resolvedHash[len(prefix):]
}
