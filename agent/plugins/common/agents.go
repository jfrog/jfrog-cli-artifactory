package common

import (
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

type AgentConfig = agentcommon.AgentConfig

type AgentSpec = agentcommon.AgentSpec

// Agents is the hardcoded set of agents currently supported by `jf agent plugins`.
// User overrides come from agent-config.json -> "plugins-agents".
var Agents = map[string]AgentConfig{
	// For Claude, Cursor, and Codex, GlobalDir is the BASE directory.
	// The Artifactory repo key is injected as a subdirectory at install time so
	// each repo gets its own isolated marketplace directory:
	//   Claude: <GlobalDir>/<repoKey>/<slug>
	//   Cursor: <GlobalDir>/<repoKey>/<slug>
	//   Codex:  <GlobalDir>/<repoKey>/<slug>
	//
	// All agents support global scope only. Project scope is not supported because:
	//   - Claude: Plugin config is user-scoped only (~/.claude/settings.json)
	//   - Cursor: Only auto-discovers full plugins from ~/.cursor/plugins/local/
	//   - Codex:  Plugin config is user-scoped only (~/.codex/config.toml)
	"claude": {GlobalDir: "~/.claude/plugins/local", ProjectDir: ""},
	"cursor": {GlobalDir: "~/.cursor/plugins/local", ProjectDir: ""},
	"codex":  {GlobalDir: "~/.agents/plugins/local", ProjectDir: ""},
}

// RegistryHelp configures agent-config.json help text for plugins harness resolution.
var RegistryHelp = agentcommon.AgentRegistryHelpExample{
	ConfigSectionKey:  agentcommon.PluginsAgentsKey,
	ExampleProjectDir: ".my-agent/plugins",
	ExampleGlobalDir:  "~/.my-agent/plugins",
}
