package common

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureDestinationDir mkdirs dest if missing, errors if path exists and is not a directory.
func EnsureDestinationDir(dest string) error {
	info, err := os.Stat(dest)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("install destination %q exists and is not a directory", dest)
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// #nosec G301 -- installed files need to be readable across the user's tools.
		if mkErr := os.MkdirAll(dest, 0750); mkErr != nil {
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

// UnzipFile extracts the zip at src into dest, rejecting entries that would escape dest.
func UnzipFile(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	if err := os.MkdirAll(dest, 0750); err != nil {
		return err
	}

	for _, entry := range reader.File {
		// #nosec G305 -- path traversal is checked immediately below.
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
		// #nosec G301 -- installed files need to be readable.
		if err := os.MkdirAll(filepath.Dir(entryPath), 0750); err != nil {
			return err
		}
		if err := extractZipFile(entry, entryPath); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(entry *zip.File, dest string) error {
	if strings.Contains(dest, "..") {
		return fmt.Errorf("illegal file path: %s", dest)
	}
	readCloser, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = readCloser.Close()
	}()

	cleanDest := filepath.Clean(dest)
	// #nosec G304 -- dest is validated in UnzipFile to be under the extraction directory.
	outFile, err := os.OpenFile(cleanDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode())
	if err != nil {
		return err
	}
	defer func() {
		_ = outFile.Close()
	}()
	// #nosec G110 -- zip files are size-bounded by Artifactory upload limits.
	_, err = io.Copy(outFile, readCloser)
	return err
}

// CopyDir recursively copies src into dst, preserving directory modes.
func CopyDir(src, dst string) error {
	// #nosec G301 -- installed files need to be readable.
	if err := os.MkdirAll(dst, 0750); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}
		return copyRegularFile(path, destPath)
	})
}

func copyRegularFile(src, dst string) error {
	// #nosec G304 -- src comes from our own unzip temp directory.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()
	// #nosec G304 -- dst is constructed from a validated unzip output path.
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	_, err = io.Copy(out, in)
	return err
}
