package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// PathAgentName is the synthetic agent name used when --path selects a single target.
const PathAgentName = "(path)"

// ScopeMode identifies the install/update target scope.
type ScopeMode string

const (
	ScopeProject ScopeMode = "project"
	ScopeGlobal  ScopeMode = "global"
	ScopePath    ScopeMode = "path"
)

// AgentTarget pairs an agent spec with the resolved absolute destination directory (includes slug).
type AgentTarget struct {
	Agent          AgentSpec
	Scope          ScopeMode
	DestinationDir string
}

// ValidateInstallFlags validates `--path | (--harness [, --project-dir | --global])` for install/update.
// absoluteInstallBaseDir is non-empty when --path was supplied; otherwise specs are resolved from --harness.
func ValidateInstallFlags(c *components.Context) (absoluteInstallBaseDir string, specs []AgentSpec, projectDirAbs string, isGlobal bool, err error) {
	pathInstallBase := strings.TrimSpace(c.GetStringFlagValue("path"))
	rawHarness := strings.TrimSpace(c.GetStringFlagValue("harness"))
	isGlobal = c.GetBoolFlagValue("global")
	projectDir := strings.TrimSpace(c.GetStringFlagValue("project-dir"))

	if pathInstallBase != "" {
		if rawHarness != "" {
			err = fmt.Errorf("--path cannot be combined with --harness")
			return
		}
		if isGlobal {
			err = fmt.Errorf("--path cannot be combined with --global")
			return
		}
		if projectDir != "" {
			err = fmt.Errorf("--path cannot be combined with --project-dir")
			return
		}
		if err = ValidateExistingDir(pathInstallBase); err != nil {
			err = fmt.Errorf("--path: %w", err)
			return
		}
		absoluteInstallBaseDir, err = filepath.Abs(pathInstallBase)
		if err != nil {
			err = fmt.Errorf("invalid --path %q: %w", pathInstallBase, err)
		}
		return
	}

	registry, err := LoadAgentRegistry()
	if err != nil {
		return
	}
	if rawHarness == "" {
		err = fmt.Errorf("--harness is required unless --path is set. Supported harnesses: %s", AgentNames(registry))
		return
	}

	harnessNames, err := ParseHarnessList(rawHarness)
	if err != nil {
		return
	}

	specs = make([]AgentSpec, 0, len(harnessNames))
	for _, name := range harnessNames {
		agentSpec, resolveErr := ResolveAgent(registry, name)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		specs = append(specs, agentSpec)
	}

	if isGlobal && projectDir != "" {
		err = fmt.Errorf("--global and --project-dir are mutually exclusive, please choose either --global or --project-dir")
		return
	}

	if !isGlobal {
		dir := projectDir
		if dir == "" {
			dir = "."
		}
		absoluteProjectDir, resolveErr := filepath.Abs(dir)
		if resolveErr != nil {
			err = fmt.Errorf("invalid --project-dir %q: %w", dir, resolveErr)
			return
		}
		info, statErr := os.Stat(absoluteProjectDir)
		if statErr != nil || !info.IsDir() {
			err = fmt.Errorf("--project-dir %q is not an existing directory", dir)
			return
		}
		projectDirAbs = absoluteProjectDir
	}
	return
}

// ResolveAgentTargets resolves per-agent install destinations.
// When path is non-empty, a single ScopePath target is returned.
func ResolveAgentTargets(slug, path string, agents []AgentSpec, projectDirAbs string, isGlobal bool) ([]AgentTarget, error) {
	if path != "" {
		base, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("invalid install path %q: %w", path, err)
		}
		return []AgentTarget{{
			Agent:          AgentSpec{Name: PathAgentName},
			Scope:          ScopePath,
			DestinationDir: filepath.Join(base, slug),
		}}, nil
	}

	scope := ScopeProject
	if isGlobal {
		scope = ScopeGlobal
	}
	if scope == ScopeProject && projectDirAbs == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}

	targets := make([]AgentTarget, 0, len(agents))
	for _, agent := range agents {
		base, err := ResolveAgentInstallDir(agent, projectDirAbs, isGlobal)
		if err != nil {
			return nil, err
		}
		targets = append(targets, AgentTarget{
			Agent:          agent,
			Scope:          scope,
			DestinationDir: filepath.Join(base, slug),
		})
	}
	return targets, nil
}
