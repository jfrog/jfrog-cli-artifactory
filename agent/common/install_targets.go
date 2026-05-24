package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// PathAgentName is the synthetic agent name used for the single target returned in --path mode.
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

// ValidateInstallFlags validates `--path | (--agent [, --project-dir | --global])` for install.
// When --path is set, absoluteInstallBaseDir is non-empty; otherwise specs are resolved from --agent.
func ValidateInstallFlags(commandContext *components.Context, packageConfig AgentPackageConfig) (absoluteInstallBaseDir string, specs []AgentSpec, projectDirAbs string, isGlobal bool, err error) {
	pathInstallBase := strings.TrimSpace(commandContext.GetStringFlagValue("path"))
	rawAgents := strings.TrimSpace(commandContext.GetStringFlagValue("agent"))
	isGlobal = commandContext.GetBoolFlagValue("global")
	projectDir := strings.TrimSpace(commandContext.GetStringFlagValue("project-dir"))

	if pathInstallBase != "" {
		if rawAgents != "" {
			err = fmt.Errorf("--path cannot be combined with --agent")
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

	registry, err := LoadAgentRegistry(packageConfig)
	if err != nil {
		return
	}
	if rawAgents == "" {
		err = fmt.Errorf("--agent is required unless --path is set. Supported agents: %s", AgentNames(registry))
		return
	}

	agentNames, err := ParseAgentList(rawAgents)
	if err != nil {
		return
	}

	specs = make([]AgentSpec, 0, len(agentNames))
	for _, name := range agentNames {
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
