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

// ParseHarnessList parses comma-separated harness names (trim, lowercase, reject empty/duplicates).
func ParseHarnessList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("--harness is required (comma-separated list of harness names)")
	}

	seen := make(map[string]struct{})
	var result []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			return nil, fmt.Errorf("--harness contains an empty name in %q", raw)
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("--harness lists %q more than once", name)
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result, nil
}
