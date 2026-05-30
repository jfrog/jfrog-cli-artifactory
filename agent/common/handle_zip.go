package common

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
