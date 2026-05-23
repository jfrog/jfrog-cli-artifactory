package common

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// zipEpoch is the earliest valid timestamp in ZIP format (MS-DOS epoch).
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

var publishZipExcludes = map[string]bool{
	".git":         true,
	".jfrog":       true,
	"__pycache__":  true,
	"node_modules": true,
	".DS_Store":    true,
}

// ZipFileEntry is a single file included in a publish bundle zip.
type ZipFileEntry struct {
	RelPath string
	Mode    os.FileMode
}

// ZipPublishOptions configures ZipPublishBundle.
type ZipPublishOptions struct {
	SourceDir      string
	Slug           string
	Version        string
	TempDirPrefix  string
	ContentLabel   string
	HashWhileWrite bool
}

// ZipPublishBundle builds a deterministic zip from sourceDir.
// When HashWhileWrite is true, sha256Hex is populated during the write.
// The caller must remove tmpDir (e.g. defer os.RemoveAll(tmpDir)) after the zip is consumed.
func ZipPublishBundle(opts ZipPublishOptions) (zipPath, tmpDir, sha256Hex string, err error) {
	if err := validatePublishVersion(opts.Version); err != nil {
		return "", "", "", err
	}

	files, uniformMtime, err := preparePublishZipFiles(opts)
	if err != nil {
		return "", "", "", err
	}

	tmpDir, err = os.MkdirTemp("", opts.TempDirPrefix)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	zipPath = publishZipOutputPath(tmpDir, opts.Slug, opts.Version)
	sha256Hex, err = writePublishZip(zipPath, opts.SourceDir, files, uniformMtime, opts.HashWhileWrite)
	return zipPath, tmpDir, sha256Hex, err
}

func validatePublishVersion(version string) error {
	return ValidateSemver(version)
}

func preparePublishZipFiles(opts ZipPublishOptions) (files []ZipFileEntry, uniformMtime time.Time, err error) {
	files, maxMtime, err := CollectPublishFiles(opts.SourceDir)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to collect %s files: %w", opts.ContentLabel, err)
	}
	if len(files) == 0 {
		return nil, time.Time{}, fmt.Errorf(
			"no files found in %s directory %s (all files may have been excluded)",
			opts.ContentLabel, opts.SourceDir,
		)
	}
	if maxMtime.IsZero() {
		maxMtime = zipEpoch
	}
	return files, maxMtime, nil
}

func publishZipOutputPath(tmpDir, slug, version string) string {
	return filepath.Clean(filepath.Join(tmpDir, fmt.Sprintf("%s-%s.zip", slug, version)))
}

func writePublishZip(
	zipPath, sourceDir string,
	files []ZipFileEntry,
	uniformMtime time.Time,
	hashWhileWrite bool,
) (sha256Hex string, err error) {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() {
		if cerr := zipFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close zip file: %w", cerr)
		}
	}()

	zipWriter, hasher := newPublishZipWriter(zipFile, hashWhileWrite)
	defer func() {
		if cerr := zipWriter.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to finalize zip: %w", cerr)
		}
		if err == nil && hashWhileWrite {
			sha256Hex = hex.EncodeToString(hasher.Sum(nil))
		}
	}()

	for _, fileEntry := range files {
		if err = addFileToZip(zipWriter, sourceDir, fileEntry, uniformMtime); err != nil {
			return "", fmt.Errorf("failed to add %s to zip: %w", fileEntry.RelPath, err)
		}
	}
	return sha256Hex, nil
}

func newPublishZipWriter(zipFile *os.File, hashWhileWrite bool) (*zip.Writer, hashWriter) {
	if !hashWhileWrite {
		return zip.NewWriter(zipFile), nil
	}
	hasher := sha256.New()
	return zip.NewWriter(io.MultiWriter(zipFile, hasher)), hasher
}

type hashWriter interface {
	io.Writer
	Sum([]byte) []byte
}

// CollectPublishFiles walks sourceDir and returns a sorted list of included files
// plus the max mtime across all included files.
func CollectPublishFiles(sourceDir string) (files []ZipFileEntry, maxMtime time.Time, err error) {
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if ShouldExcludePublishPath(relPath, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			files = append(files, ZipFileEntry{RelPath: relPath, Mode: info.Mode()})
			if info.ModTime().After(maxMtime) {
				maxMtime = info.ModTime()
			}
		}
		return nil
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files, maxMtime, nil
}

func addFileToZip(zipWriter *zip.Writer, sourceDir string, fileEntry ZipFileEntry, uniformTime time.Time) error {
	absPath := filepath.Join(sourceDir, fileEntry.RelPath)

	header := &zip.FileHeader{
		Name:     fileEntry.RelPath,
		Method:   zip.Deflate,
		Modified: uniformTime,
	}
	header.SetModTime(uniformTime) //nolint:staticcheck // sets legacy MS-DOS ModifiedDate/ModifiedTime fields
	header.SetMode(normalizeZipFileMode(fileEntry.Mode))
	header.Extra = nil

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	// #nosec G304 -- absPath is from a user-provided directory joined with a walked relative path.
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close() // read-side close after copy
	}()

	_, err = io.Copy(writer, file)
	return err
}

func normalizeZipFileMode(mode os.FileMode) os.FileMode {
	if runtime.GOOS == "windows" {
		return DefaultFileMode
	}
	return mode
}

// ShouldExcludePublishPath reports whether a walked path should be omitted from publish zips.
func ShouldExcludePublishPath(relPath string, info os.FileInfo) bool {
	name := info.Name()

	if publishZipExcludes[name] {
		return true
	}
	if strings.HasSuffix(name, ".pyc") {
		return true
	}
	return false
}

// ComputeSHA256 returns the hex-encoded SHA256 digest of the file at path.
// Callers pass paths from ZipPublishBundle or PrebuiltPublishZipPath (typically absolute).
func ComputeSHA256(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if err := validateReadableFilePath(cleanPath); err != nil {
		return "", err
	}
	// #nosec G304 -- cleanPath is validated and produced by this package's publish flow.
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close() // read-side close after hash
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// validateReadableFilePath guards os.Open for gosec. filepath.IsLocal applies to
// relative paths only (it rejects absolute paths). Publish zips from MkdirTemp are absolute.
func validateReadableFilePath(path string) error {
	if filepath.IsAbs(path) {
		return nil
	}
	if filepath.IsLocal(path) {
		return nil
	}
	return fmt.Errorf("invalid path %q", path)
}

// IsPrebuiltPublishZip reports whether sourceDir/zip/{slug}_{version}.zip exists.
func IsPrebuiltPublishZip(sourceDir, slug, version string) bool {
	prebuilt := filepath.Join(sourceDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version))
	_, err := os.Stat(prebuilt)
	return err == nil
}

// PrebuiltPublishZipPath returns the canonical path to a prebuilt publish zip.
func PrebuiltPublishZipPath(sourceDir, slug, version string) string {
	return filepath.Clean(filepath.Join(sourceDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version)))
}
