package common

import (
	"fmt"
	"path/filepath"
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

// ResolveAgentTargets resolves per-agent install destinations for a slug.
func ResolveAgentTargets(slug, path string, agents []AgentSpec, projectDirAbs string, isGlobal bool) ([]InstallTarget, error) {
	if path != "" {
		target, err := BuildPathInstallTarget(slug, path)
		if err != nil {
			return nil, err
		}
		return []InstallTarget{target}, nil
	}

	scope := InstallScopeProject
	if isGlobal {
		scope = InstallScopeGlobal
	}
	if scope == InstallScopeProject && projectDirAbs == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}

	targets := make([]InstallTarget, 0, len(agents))
	for _, agent := range agents {
		base, err := ResolveAgentInstallDir(agent, projectDirAbs, isGlobal)
		if err != nil {
			return nil, err
		}
		targets = append(targets, InstallTarget{
			Agent:          agent,
			Scope:          scope,
			DestinationDir: filepath.Join(base, slug),
		})
	}
	return targets, nil
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
