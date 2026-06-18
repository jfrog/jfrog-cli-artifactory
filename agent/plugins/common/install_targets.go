package common

import (
	"fmt"
	"path/filepath"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

const PathAgentName = agentcommon.PathAgentName

type ScopeMode = agentcommon.InstallScope

const (
	ScopeProject = agentcommon.InstallScopeProject
	ScopeGlobal  = agentcommon.InstallScopeGlobal
	ScopePath    = agentcommon.InstallScopePath
)

type AgentTarget = agentcommon.InstallTarget

// HandleEvidenceVerification runs evidence verification for a plugin using plugin-specific policy.
func HandleEvidenceVerification(quiet bool, slug string, verifyFn func() error) error {
	return agentcommon.HandleEvidenceVerification(quiet, slug, ArtifactKind, verifyFn,
		agentcommon.ShouldFailOnMissingEvidenceForPlugins,
		agentcommon.DisableQuietFailureEvidenceHintForPlugins)
}

// ResolveAgentTargets resolves per-agent install destinations for a plugin.
// When path is non-empty, a single ScopePath target is returned.
func ResolveAgentTargets(slug, path string, agents []AgentSpec, projectDirAbs string, isGlobal bool) ([]AgentTarget, error) {
	if path != "" {
		target, err := agentcommon.BuildPathInstallTarget(slug, path)
		if err != nil {
			return nil, err
		}
		return []AgentTarget{target}, nil
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
		base, err := agentcommon.ResolveAgentInstallDir(agent, projectDirAbs, isGlobal)
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
