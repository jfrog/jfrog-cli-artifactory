package cargo

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// crateRepoPath parses "<name>-<version>.crate" into the Artifactory path and parts.
// The layout matches how Artifactory stores Cargo crates (verified live against a Cargo repo and
// per JFrog docs): "crates/<name>/<name>-<version>.crate". The version is the substring after the
// LAST hyphen that begins a semver-looking token.
func crateRepoPath(fileName string) (path, name, version string) {
	base := strings.TrimSuffix(fileName, ".crate")
	// version starts at the last '-' followed by a digit
	idx := -1
	for i := 0; i < len(base)-1; i++ {
		if base[i] == '-' && base[i+1] >= '0' && base[i+1] <= '9' {
			idx = i
		}
	}
	if idx == -1 {
		return "crates/" + base + "/" + fileName, base, ""
	}
	name = base[:idx]
	version = base[idx+1:]
	return "crates/" + name + "/" + fileName, name, version
}

// scanCrateArtifacts finds built .crate files under target/package and builds artifacts.
func scanCrateArtifacts(workingDir, repo string) ([]entities.Artifact, error) {
	pkgDir := filepath.Join(workingDir, "target", "package")
	matches, err := filepath.Glob(filepath.Join(pkgDir, "*.crate"))
	if err != nil {
		return nil, fmt.Errorf("scan crate artifacts: %w", err)
	}
	var arts []entities.Artifact
	for _, file := range matches {
		fileName := filepath.Base(file)
		repoPath, _, _ := crateRepoPath(fileName)
		art := entities.Artifact{
			Name:                   fileName,
			Path:                   repoPath,
			Type:                   "crate",
			OriginalDeploymentRepo: repo,
		}
		if fd, derr := crypto.GetFileDetails(file, true); derr == nil {
			art.Checksum = entities.Checksum{Sha1: fd.Checksum.Sha1, Sha256: fd.Checksum.Sha256, Md5: fd.Checksum.Md5}
		} else {
			log.Debug("cargo: could not checksum " + file + ": " + derr.Error())
		}
		arts = append(arts, art)
	}
	return arts, nil
}
