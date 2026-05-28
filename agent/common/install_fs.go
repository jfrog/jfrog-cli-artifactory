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
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = r.Close()
	}()

	if err := os.MkdirAll(dest, InstallDirMode); err != nil {
		return err
	}

	for _, f := range r.File {
		// #nosec G305 -- path traversal is checked immediately below
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		// #nosec G301 -- install files need to be readable
		if err := os.MkdirAll(filepath.Dir(fpath), InstallDirMode); err != nil {
			return err
		}

		if err := extractFile(f, fpath); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, dest string) error {
	if strings.Contains(dest, "..") {
		return fmt.Errorf("illegal file path: %s", dest)
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	cleanDest := filepath.Clean(dest)
	// #nosec G304 -- dest is validated in UnzipFile and above to be under the extraction directory
	outFile, err := os.OpenFile(cleanDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer func() {
		_ = outFile.Close()
	}()

	// #nosec G110 -- zip files are size-bounded by Artifactory upload limits
	_, err = io.Copy(outFile, rc)
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
		_ = in.Close()
	}()

	// #nosec G304 -- dst is constructed from validated unzip output path
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
