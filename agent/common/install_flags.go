package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// InstallFlagInput holds install flag values shared by skills and plugins install validation.
type InstallFlagInput struct {
	PathInstallBase string
	RawHarness      string
	ProjectDir      string
	IsGlobal        bool
}

// InstallFlagsResult holds validated install/update flags after harness or --path resolution.
type InstallFlagsResult struct {
	// AbsoluteInstallBaseDir is set when --path was used; empty in harness mode.
	AbsoluteInstallBaseDir string
	// Specs lists resolved agents when --harness was used; empty in path mode.
	Specs []AgentSpec
	// ProjectDirAbs is the absolute project root for project-scoped harness installs.
	ProjectDirAbs string
	IsGlobal      bool
}

// PathMode reports whether install used --path instead of --harness.
func (r InstallFlagsResult) PathMode() bool {
	return r.AbsoluteInstallBaseDir != ""
}

// ResolvePathInstallBase validates --path install mode and returns the absolute base directory.
// An empty PathInstallBase means harness mode; callers should continue with harness resolution.
func ResolvePathInstallBase(flags InstallFlagInput) (string, error) {
	if flags.PathInstallBase == "" {
		return "", nil
	}
	if flags.RawHarness != "" {
		return "", fmt.Errorf("--path cannot be combined with --harness")
	}
	if flags.IsGlobal {
		return "", fmt.Errorf("--path cannot be combined with --global")
	}
	if flags.ProjectDir != "" {
		return "", fmt.Errorf("--path cannot be combined with --project-dir")
	}
	if err := ValidateExistingDir(flags.PathInstallBase); err != nil {
		return "", fmt.Errorf("--path: %w", err)
	}
	absPath, err := filepath.Abs(flags.PathInstallBase)
	if err != nil {
		return "", fmt.Errorf("invalid --path %q: %w", flags.PathInstallBase, err)
	}
	return absPath, nil
}

// ResolveInstallProjectDir validates --project-dir for harness install mode (skipped when --global).
func ResolveInstallProjectDir(projectDir string, isGlobal bool) (string, error) {
	if isGlobal && projectDir != "" {
		return "", fmt.Errorf("--global and --project-dir are mutually exclusive, please choose either --global or --project-dir")
	}
	if isGlobal {
		return "", nil
	}
	dir := projectDir
	if dir == "" {
		dir = "."
	}
	absoluteProjectDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid --project-dir %q: %w", dir, err)
	}
	info, err := os.Stat(absoluteProjectDir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return "", fmt.Errorf("--project-dir %q exists but is not a directory", dir)
		}
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("--project-dir %q does not exist", dir)
	default:
		return "", fmt.Errorf("cannot access --project-dir %q: %w", dir, err)
	}
	return absoluteProjectDir, nil
}
