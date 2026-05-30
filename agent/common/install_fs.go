package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

const (
	jfrogInstallDirName = ".jfrog"

	// PackageDownloadThreads is the parallel download count for package and marketplace files.
	PackageDownloadThreads = 1
	// PackageDownloadRetries is the HTTP retry count for package and marketplace downloads.
	PackageDownloadRetries = 3
	// InstallInfoManifestSchemaVersion is bumped when the JSON shape changes incompatibly.
	InstallInfoManifestSchemaVersion = 1
)

// InstallInfoManifest is CLI-owned metadata for an installed skill or plugin.
type InstallInfoManifest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Repo             string `json:"repo"`
	Slug             string `json:"slug"`
	InstalledVersion string `json:"installedVersion"`
	Scope            string `json:"scope"`
	Agent            string `json:"agent"`
	ProjectDir       string `json:"projectDir,omitempty"`
}

// installInfoManifestPath is <installDir>/.jfrog/<manifestFileName>.
func installInfoManifestPath(installDir, manifestFileName string) string {
	return filepath.Join(installDir, jfrogInstallDirName, manifestFileName)
}

// WriteInstallInfoManifest writes the manifest under installDir (.jfrog/<manifestFileName>).
func WriteInstallInfoManifest(installDir, manifestFileName string, manifest InstallInfoManifest) error {
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = InstallInfoManifestSchemaVersion
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal install info manifest: %w", err)
	}
	path := installInfoManifestPath(installDir, manifestFileName)
	if err := os.MkdirAll(filepath.Dir(path), InstallDirMode); err != nil {
		return fmt.Errorf("create .jfrog under install dir: %w", err)
	}
	// #nosec G306 -- manifest lives under user-owned install dir.
	if err := os.WriteFile(path, data, InstallManifestFileMode); err != nil {
		return fmt.Errorf("write install info manifest: %w", err)
	}
	return nil
}

// ReadInstallInfoManifest reads .jfrog/<manifestFileName> when present.
// A missing file returns (nil, nil).
func ReadInstallInfoManifest(installDir, manifestFileName string) (*InstallInfoManifest, error) {
	path := installInfoManifestPath(installDir, manifestFileName)
	// #nosec G304 -- path is install directory joined with fixed .jfrog segments.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read install manifest: %w", err)
	}
	var manifest InstallInfoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse install manifest %s: %w", path, err)
	}
	return &manifest, nil
}

// packageZipDownloadManager downloads package zips from Artifactory; swappable in tests.
type packageZipDownloadManager interface {
	DownloadFiles(params ...services.DownloadParams) (totalDownloaded, totalFailed int, err error)
}

var createDownloadServiceManagerForPackageZip = func(serverDetails *config.ServerDetails) (packageZipDownloadManager, error) {
	return utils.CreateDownloadServiceManager(serverDetails, PackageDownloadThreads, PackageDownloadRetries, 0, false, nil)
}

// DownloadPackageZip downloads <repo>/<slug>/<version>/<slug>-<version>.zip into tmpDir.
func DownloadPackageZip(serverDetails *config.ServerDetails, repoKey, slug, version, tmpDir, artifactKind string) (string, error) {
	serviceManager, err := createDownloadServiceManagerForPackageZip(serverDetails)
	if err != nil {
		return "", fmt.Errorf("create download service manager: %w", err)
	}

	pattern := fmt.Sprintf("%s/%s/%s/%s-%s.zip", repoKey, slug, version, slug, version)
	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = pattern
	downloadParams.Target = tmpDir + "/"
	downloadParams.Flat = true

	totalDownloaded, totalFailed, err := serviceManager.DownloadFiles(downloadParams)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", pattern, err)
	}
	if totalFailed > 0 {
		return "", fmt.Errorf("download failed for %s", pattern)
	}
	if totalDownloaded == 0 {
		return "", fmt.Errorf("%s '%s' version '%s' not found in repository '%s'", artifactKind, slug, version, repoKey)
	}

	zipName := fmt.Sprintf("%s-%s.zip", slug, version)
	return filepath.Join(tmpDir, zipName), nil
}

// verifyEvidenceForPackageZip is swappable in tests.
var verifyEvidenceForPackageZip = VerifyEvidence

// VerifyPackageEvidence verifies evidence for a published package zip in Artifactory.
func VerifyPackageEvidence(serverDetails *config.ServerDetails, repoKey, slug, version string) error {
	if repoKey == "" || slug == "" || version == "" {
		return fmt.Errorf("cannot verify evidence: repoKey, slug, and version must all be set")
	}
	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", repoKey, slug, version, slug, version)
	return verifyEvidenceForPackageZip(serverDetails, VerifyEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
	})
}
