package common

import (
	"fmt"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

type AgentConfig = agentcommon.AgentConfig

type AgentSpec = agentcommon.AgentSpec

// Agents is built-in defaults; merged with ~/.jfrog/agents/agent-config.json.
var Agents = map[string]AgentConfig{
	"claude-code":    {GlobalDir: "~/.claude/skills", ProjectDir: ".claude/skills"},
	"cursor":         {GlobalDir: "~/.cursor/skills", ProjectDir: ".cursor/skills"},
	"github-copilot": {GlobalDir: "~/.copilot/skills", ProjectDir: ".github/skills"},
	"windsurf":       {GlobalDir: "~/.codeium/windsurf/skills", ProjectDir: ".windsurf/skills"},
	"codex":          {GlobalDir: "~/.codex/skills", ProjectDir: ".codex/skills"},
	"cross-agent":    {GlobalDir: "~/.agents/skills", ProjectDir: ".agents/skills"},
}

// RegistryHelp configures agent-config.json help text for skills harness resolution.
var RegistryHelp = agentcommon.AgentRegistryHelpExample{
	ConfigSectionKey:  agentcommon.SkillsAgentsKey,
	ExampleProjectDir: ".my-agent/skills",
	ExampleGlobalDir:  "~/.my-agent/skills",
}

// ParseHarnessForList parses --harness for list (exactly one harness name; commas are rejected).
func ParseHarnessForList(raw string) (string, error) {
	names, err := agentcommon.ParseHarnessList(raw)
	if err != nil {
		return "", err
	}
	if len(names) != 1 {
		return "", fmt.Errorf("--harness for list accepts one harness name, not a comma-separated list: %q", raw)
	}
	return names[0], nil
}
