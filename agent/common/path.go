package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathAgentName is the synthetic agent name used when --path selects the install target.
const PathAgentName = "(path)"

// InstallScope identifies the install/update target scope.
type InstallScope string

const (
	InstallScopeProject InstallScope = "project"
	InstallScopeGlobal  InstallScope = "global"
	InstallScopePath    InstallScope = "path"
)

// InstallTarget pairs an agent spec with the resolved absolute destination directory (includes slug).
type InstallTarget struct {
	Agent          AgentSpec
	Scope          InstallScope
	DestinationDir string
}

// BuildPathInstallTarget returns a ScopePath install target at path/slug.
func BuildPathInstallTarget(slug, path string) (InstallTarget, error) {
	base, err := filepath.Abs(path)
	if err != nil {
		return InstallTarget{}, fmt.Errorf("invalid install path %q: %w", path, err)
	}
	return InstallTarget{
		Agent:          AgentSpec{Name: PathAgentName},
		Scope:          InstallScopePath,
		DestinationDir: filepath.Join(base, slug),
	}, nil
}

// ValidateExistingDir requires path to exist and be a directory (after filepath.Abs).
func ValidateExistingDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	return nil
}

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

// InstallFlagInput holds install flag values shared by skills and plugins install validation.
type InstallFlagInput struct {
	PathInstallBase string
	RawHarness      string
	ProjectDir      string
	IsGlobal        bool
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
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("--project-dir %q is not an existing directory", dir)
	}
	return absoluteProjectDir, nil
}
