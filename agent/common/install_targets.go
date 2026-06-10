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
