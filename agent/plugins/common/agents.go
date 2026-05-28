package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

type AgentConfig = agentcommon.AgentConfig

type AgentSpec = agentcommon.AgentSpec

// Agents is the hardcoded set of agents currently supported by `jf agent plugins`.
// User overrides come from agent-config.json -> "plugins-agents".
var Agents = map[string]AgentConfig{
	"claude": {GlobalDir: "~/.claude/plugins", ProjectDir: ".claude/plugins"},
	"cursor": {GlobalDir: "~/.cursor/plugins", ProjectDir: ".cursor/plugins"},
	"codex":  {GlobalDir: "~/.codex/plugins", ProjectDir: ".codex/plugins"},
}

// RegistryHelp configures agent-config.json help text for plugins harness resolution.
var RegistryHelp = agentcommon.AgentRegistryHelpExample{
	ConfigSectionKey:  agentcommon.PluginsAgentsKey,
	ExampleProjectDir: ".my-agent/plugins",
	ExampleGlobalDir:  "~/.my-agent/plugins",
}

// ParseSingleHarness parses a single harness name from --harness. Comma-separated lists
// are rejected because plugins install targets exactly one agent.
func ParseSingleHarness(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("--harness is required (single harness name)")
	}
	if strings.Contains(trimmed, ",") {
		return "", fmt.Errorf("--harness for plugins install accepts a single harness name, not a comma-separated list: %q", raw)
	}
	return strings.ToLower(trimmed), nil
}
