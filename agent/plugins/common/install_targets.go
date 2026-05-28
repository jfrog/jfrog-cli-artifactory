package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// PathAgentName is the synthetic agent name used when --path selects the install target.
const PathAgentName = "(path)"

// ScopeMode identifies the install target scope.
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

// ValidateInstallFlags validates `--path | (--harness [, --project-dir | --global])` for plugins install.
// absoluteInstallBaseDir is non-empty when --path was supplied; otherwise spec is resolved from --harness.
// Unlike skills install, --harness accepts exactly one agent name.
func ValidateInstallFlags(c *components.Context) (absoluteInstallBaseDir string, spec AgentSpec, projectDirAbs string, isGlobal bool, err error) {
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
		if err = agentcommon.ValidateExistingDir(pathInstallBase); err != nil {
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

	harnessName, err := ParseSingleHarness(rawHarness)
	if err != nil {
		return
	}

	spec, err = ResolveAgent(registry, harnessName)
	if err != nil {
		return
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

// ResolveAgentTarget returns the single install destination for a plugin.
// When path is non-empty, a ScopePath target is returned; otherwise the
// agent's project/global directory is used.
func ResolveAgentTarget(slug, path string, spec AgentSpec, projectDirAbs string, isGlobal bool) (AgentTarget, error) {
	if path != "" {
		base, err := filepath.Abs(path)
		if err != nil {
			return AgentTarget{}, fmt.Errorf("invalid install path %q: %w", path, err)
		}
		return AgentTarget{
			Agent:          AgentSpec{Name: PathAgentName},
			Scope:          ScopePath,
			DestinationDir: filepath.Join(base, slug),
		}, nil
	}

	scope := ScopeProject
	if isGlobal {
		scope = ScopeGlobal
	}
	if scope == ScopeProject && projectDirAbs == "" {
		return AgentTarget{}, fmt.Errorf("project directory is required for project-scoped install")
	}

	base, err := ResolveAgentInstallDir(spec, projectDirAbs, isGlobal)
	if err != nil {
		return AgentTarget{}, err
	}
	return AgentTarget{
		Agent:          spec,
		Scope:          scope,
		DestinationDir: filepath.Join(base, slug),
	}, nil
}
