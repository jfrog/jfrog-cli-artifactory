package common

import (
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

// PluginConfigFileName is the JSON file (under ~/.jfrog/agents/) that holds plugin-specific
// agent overrides and additional plugin.json search paths.
const PluginConfigFileName = "agent-plugin-config.json"

// Agents is the built-in default plugin directories for each AI agent. They are merged with
// the user's agent-plugin-config.json (JSON entries win on collision).
var Agents = map[string]agentcommon.AgentConfig{
	"claude-code":    {GlobalDir: "~/.claude/plugins", ProjectDir: ".claude/plugins"},
	"cursor":         {GlobalDir: "~/.cursor/plugins", ProjectDir: ".cursor/plugins"},
	"github-copilot": {GlobalDir: "~/.copilot/plugins", ProjectDir: ".github/plugins"},
	"windsurf":       {GlobalDir: "~/.codeium/windsurf/plugins", ProjectDir: ".windsurf/plugins"},
	"codex":          {GlobalDir: "~/.codex/plugins", ProjectDir: ".codex/plugins"},
	"cross-agent":    {GlobalDir: "~/.agents/plugins", ProjectDir: ".agents/plugins"},
}

// PackageConfig returns the AgentPackageConfig used by shared helpers in agent/common
// (registry loader, install-targets validator, supported-agents list).
func PackageConfig() agentcommon.AgentPackageConfig {
	return agentcommon.AgentPackageConfig{
		ConfigFileName: PluginConfigFileName,
		Defaults:       Agents,
	}
}

// LoadAgentRegistry merges agent-plugin-config.json over the built-in plugin defaults.
func LoadAgentRegistry() (map[string]agentcommon.AgentSpec, error) {
	return agentcommon.LoadAgentRegistry(PackageConfig())
}

// SupportedAgentsList returns the comma-separated agent names for help text.
func SupportedAgentsList() string {
	return agentcommon.SupportedAgentsList(PackageConfig())
}
