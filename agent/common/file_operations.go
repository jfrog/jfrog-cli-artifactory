package common

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome maps a leading "~/" to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// ValidateExistingDir requires path to exist and be a directory (after filepath.Abs).
func ValidateExistingDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return existingDirStatError(path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("directory %q exists but is not a directory", path)
	}
	return nil
}

func existingDirStatError(path string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("directory %q does not exist", path)
	}
	return fmt.Errorf("cannot access directory %q: %w", path, err)
}

// EnsureDestinationDir mkdirs the path if missing; errors when the path exists and is not a directory.
func EnsureDestinationDir(dest string) error {
	info, err := os.Stat(dest)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("install destination %q exists and is not a directory", dest)
		}
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

// CopyFile copies the contents of src to dst, creating or truncating dst.
func CopyFile(src, dst string) error {
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

		return CopyFile(path, destPath)
	})
}

// MovePath renames src to dst.
func MovePath(src, dst string) error {
	return os.Rename(src, dst)
}

// RemovePath deletes a file or directory tree at path.
func RemovePath(path string) error {
	return os.RemoveAll(path)
}
