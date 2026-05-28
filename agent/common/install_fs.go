package common

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

const jfrogInstallDirName = ".jfrog"

const (
	// PackageDownloadThreads is the parallel download count for package and marketplace files.
	PackageDownloadThreads = 1
	// PackageDownloadRetries is the HTTP retry count for package and marketplace downloads.
	PackageDownloadRetries = 3
)

// InstallInfoManifestSchemaVersion is bumped when the JSON shape changes incompatibly.
const InstallInfoManifestSchemaVersion = 1

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

// InstallInfoManifestPath returns the path to the manifest under an install directory.
func InstallInfoManifestPath(installDir, manifestFileName string) string {
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
	dir := filepath.Join(installDir, jfrogInstallDirName)
	if err := os.MkdirAll(dir, InstallDirMode); err != nil {
		return fmt.Errorf("create .jfrog under install dir: %w", err)
	}
	path := filepath.Join(dir, manifestFileName)
	// #nosec G306 -- manifest lives under user-owned install dir.
	if err := os.WriteFile(path, data, InstallManifestFileMode); err != nil {
		return fmt.Errorf("write install info manifest: %w", err)
	}
	return nil
}

// ReadInstallInfoManifest reads .jfrog/<manifestFileName> when present.
// A missing file returns (nil, nil).
func ReadInstallInfoManifest(installDir, manifestFileName string) (*InstallInfoManifest, error) {
	path := InstallInfoManifestPath(installDir, manifestFileName)
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

// EnsureDestinationDir mkdirs the path if missing; errors when the path exists and is not a directory.
func EnsureDestinationDir(dest string) error {
	info, err := os.Stat(dest)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("install destination %q exists and is not a directory", dest)
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// #nosec G301 -- install files need to be readable across the user's tools.
		if mkErr := os.MkdirAll(dest, InstallDirMode); mkErr != nil {
			return fmt.Errorf(
				"failed to create install destination %q: %w. "+
					"Create the directory at that path (including parent folders if needed), then run the command again",
				dest, mkErr,
			)
		}
		return nil
	default:
		return fmt.Errorf("install destination %q is not accessible: %w", dest, err)
	}
}

// UnzipFile extracts src into dest, rejecting entries that escape the destination directory.
func UnzipFile(src, dest string) error {
	zipReader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		// Best-effort close after zip read.
		_ = zipReader.Close()
	}()

	if err := os.MkdirAll(dest, InstallDirMode); err != nil {
		return err
	}

	for _, entry := range zipReader.File {
		// #nosec G305 -- path traversal is checked immediately below
		entryPath := filepath.Join(dest, entry.Name)

		if !strings.HasPrefix(filepath.Clean(entryPath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", entry.Name)
		}

		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(entryPath, entry.Mode()); err != nil {
				return err
			}
			continue
		}

		// #nosec G301 -- install files need to be readable
		if err := os.MkdirAll(filepath.Dir(entryPath), InstallDirMode); err != nil {
			return err
		}

		if err := extractZipEntry(entry, entryPath); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(entry *zip.File, outputPath string) error {
	if strings.Contains(outputPath, "..") {
		return fmt.Errorf("illegal file path: %s", outputPath)
	}

	entryReader, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() {
		// Best-effort close after zip entry read.
		_ = entryReader.Close()
	}()

	cleanOutputPath := filepath.Clean(outputPath)
	// #nosec G304 -- outputPath is validated in UnzipFile and above to be under the extraction directory
	outputFile, err := os.OpenFile(cleanOutputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode())
	if err != nil {
		return err
	}
	defer func() {
		// Best-effort close after extract write.
		_ = outputFile.Close()
	}()

	// #nosec G110 -- zip files are size-bounded by Artifactory upload limits
	_, err = io.Copy(outputFile, entryReader)
	return err
}

// CopyDir copies the directory tree rooted at src into dst, creating dst if needed.
func CopyDir(src, dst string) error {
	// #nosec G301 -- install files need to be readable
	if err := os.MkdirAll(dst, InstallDirMode); err != nil {
		return err
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	// #nosec G304 -- src comes from a vetted unzip temp directory
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		// Best-effort close after copy read.
		_ = in.Close()
	}()

	// #nosec G304 -- dst is constructed from validated unzip output path
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		// Best-effort close after copy write.
		_ = out.Close()
	}()

	_, err = io.Copy(out, in)
	return err
}
