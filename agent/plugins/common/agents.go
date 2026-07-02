package common

import (
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

type AgentConfig = agentcommon.AgentConfig

type AgentSpec = agentcommon.AgentSpec

// Agents is the hardcoded set of agents currently supported by `jf agent plugins`.
// User overrides come from agent-config.json -> "plugins-agents".
var Agents = map[string]AgentConfig{
	// For Claude and Codex, GlobalDir and ProjectDir are BASE directories.
	// The Artifactory repo key is injected as a subdirectory at install time so
	// each repo gets its own isolated marketplace directory:
	//   Claude: <GlobalDir>/<repoKey>/<slug>
	//   Codex:  <GlobalDir>/<repoKey>/plugins/<slug>
	//
	// Cursor project-scope installs are unsupported because .cursor/skills/ only
	// loads SKILL.md-based skills, not full plugins.
	"claude": {GlobalDir: "~/.claude/plugins/local", ProjectDir: ".claude/plugins"},
	"cursor": {GlobalDir: "~/.cursor/plugins/local", ProjectDir: ""},
	"codex":  {GlobalDir: "~/.agents/marketplaces", ProjectDir: ".agents/marketplaces"},
}

// RegistryHelp configures agent-config.json help text for plugins harness resolution.
var RegistryHelp = agentcommon.AgentRegistryHelpExample{
	ConfigSectionKey:  agentcommon.PluginsAgentsKey,
	ExampleProjectDir: ".my-agent/plugins",
	ExampleGlobalDir:  "~/.my-agent/plugins",
}
