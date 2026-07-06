package common

import (
	"path/filepath"
	"strings"

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

// InjectRepoKey rewrites the DestinationDir of Claude and Codex targets to
// include the Artifactory repo key as a subdirectory. This gives each repo its
// own isolated marketplace directory so installs from different repos never
// overwrite each other's marketplace registration.
//
//	Claude: <GlobalDir>/<slug>  →  <GlobalDir>/<repoKey>/<slug>
//	Codex:  <GlobalDir>/<slug>  →  <GlobalDir>/<repoKey>/<slug>
//
// Cursor and --path targets are returned unchanged.
func InjectRepoKey(targets []AgentTarget, repoKey string) []AgentTarget {
	result := make([]AgentTarget, len(targets))
	for i, t := range targets {
		switch strings.ToLower(t.Agent.Name) {
		case "claude":
			base := filepath.Dir(t.DestinationDir)
			slug := filepath.Base(t.DestinationDir)
			t.DestinationDir = filepath.Join(base, repoKey, slug)
		case "codex":
			base := filepath.Dir(t.DestinationDir)
			slug := filepath.Base(t.DestinationDir)
			t.DestinationDir = filepath.Join(base, repoKey, slug)
		}
		result[i] = t
	}
	return result
}
