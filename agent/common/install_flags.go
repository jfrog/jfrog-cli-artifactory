package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
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

// ValidateInstallFlags validates `--path | (--harness [, --project-dir | --global])` for install/update.
func ValidateInstallFlags(c *components.Context, builtIns map[string]AgentConfig, configSectionKey string, helpExample AgentRegistryHelpExample) (InstallFlagsResult, error) {
	input := InstallFlagInput{
		PathInstallBase: strings.TrimSpace(c.GetStringFlagValue("path")),
		RawHarness:      strings.TrimSpace(c.GetStringFlagValue("harness")),
		ProjectDir:      strings.TrimSpace(c.GetStringFlagValue("project-dir")),
		IsGlobal:        c.GetBoolFlagValue("global"),
	}
	if result, done, err := validatePathInstallFlags(input); done {
		return result, err
	}
	return validateHarnessInstallFlags(input, builtIns, configSectionKey, helpExample)
}

// validatePathInstallFlags resolves --path mode when set; done is true when validation finished or failed.
func validatePathInstallFlags(input InstallFlagInput) (result InstallFlagsResult, done bool, err error) {
	absoluteInstallBaseDir, err := ResolvePathInstallBase(input)
	if err != nil {
		return InstallFlagsResult{}, true, err
	}
	if absoluteInstallBaseDir == "" {
		return InstallFlagsResult{}, false, nil
	}
	return InstallFlagsResult{AbsoluteInstallBaseDir: absoluteInstallBaseDir}, true, nil
}

// validateHarnessInstallFlags resolves --harness targets and project/global scope.
func validateHarnessInstallFlags(input InstallFlagInput, builtIns map[string]AgentConfig, configSectionKey string, helpExample AgentRegistryHelpExample) (InstallFlagsResult, error) {
	registry, err := LoadAgentRegistry(builtIns, configSectionKey)
	if err != nil {
		return InstallFlagsResult{}, err
	}
	if err := requireHarnessWhenNotPath(input.RawHarness, registry); err != nil {
		return InstallFlagsResult{}, err
	}

	specs, err := resolveHarnessSpecs(registry, input.RawHarness, helpExample)
	if err != nil {
		return InstallFlagsResult{}, err
	}

	projectDirAbs, err := ResolveInstallProjectDir(input.ProjectDir, input.IsGlobal)
	if err != nil {
		return InstallFlagsResult{}, err
	}
	return InstallFlagsResult{
		Specs:         specs,
		ProjectDirAbs: projectDirAbs,
		IsGlobal:      input.IsGlobal,
	}, nil
}

// requireHarnessWhenNotPath ensures --harness is present when not using --path.
func requireHarnessWhenNotPath(rawHarness string, registry map[string]AgentSpec) error {
	if rawHarness != "" {
		return nil
	}
	return fmt.Errorf("--harness is required unless --path is set. Supported harnesses: %s", AgentNames(registry))
}

// resolveHarnessSpecs parses --harness and resolves each name against the agent registry.
func resolveHarnessSpecs(registry map[string]AgentSpec, rawHarness string, helpExample AgentRegistryHelpExample) ([]AgentSpec, error) {
	harnessNames, err := ParseHarnessList(rawHarness)
	if err != nil {
		return nil, err
	}

	specs := make([]AgentSpec, 0, len(harnessNames))
	for _, name := range harnessNames {
		agentSpec, resolveErr := ResolveAgent(registry, name, helpExample)
		if resolveErr != nil {
			return nil, resolveErr
		}
		specs = append(specs, agentSpec)
	}
	return specs, nil
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
