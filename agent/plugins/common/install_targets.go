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

// UsesRepoKeyedLayout reports whether agentName nests installed plugins one level deeper
// under a repo-key subdirectory (see MarketplaceRootForAgent) instead of storing them
// directly under GlobalDir. True only for claude and codex, so each Artifactory repo gets
// its own isolated marketplace subdirectory; cursor/--path have no such nesting.
func UsesRepoKeyedLayout(agentName string) bool {
	switch strings.ToLower(agentName) {
	case "claude", "codex":
		return true
	default:
		return false
	}
}

// MarketplaceRootForAgent returns the directory holding an agent's installed plugin slugs
// directly as subdirectories (filepath.Join(result, slug) is the plugin's install dir).
// For claude/codex that's <globalOrProjectDir>/<repoKey>, so repos never collide; every
// other agent uses <globalOrProjectDir> unchanged and ignores repoKey.
func MarketplaceRootForAgent(agentName, globalOrProjectDir, repoKey string) string {
	if UsesRepoKeyedLayout(agentName) {
		return filepath.Join(globalOrProjectDir, repoKey)
	}
	return globalOrProjectDir
}

// InjectRepoKey rewrites claude/codex targets' DestinationDir to
// <GlobalDir>/<slug> → <GlobalDir>/<repoKey>/<slug>, so different repos never overwrite
// each other's marketplace registration; other targets are unchanged. Callers needing the
// marketplace root before the slug is known should use MarketplaceRootForAgent directly.
func InjectRepoKey(targets []AgentTarget, repoKey string) []AgentTarget {
	result := make([]AgentTarget, len(targets))
	for i, t := range targets {
		base := filepath.Dir(t.DestinationDir)
		slug := filepath.Base(t.DestinationDir)
		t.DestinationDir = filepath.Join(MarketplaceRootForAgent(t.Agent.Name, base, repoKey), slug)
		result[i] = t
	}
	return result
}
