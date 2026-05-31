package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

type AgentConfig = agentcommon.AgentConfig

type AgentSpec = agentcommon.AgentSpec

// Agents is built-in defaults; merged with ~/.jfrog/agents/agent-config.json.
// Reference: https://github.com/vercel-labs/skills/pull/76/changes#diff-b335630551682c19a781afebcf4d07bf978fb1f8ac04c6bf87428ed5106870f5R172
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

// ParseHarnessForList parses --harness for list (exactly one harness name; commas are rejected).
func ParseHarnessForList(raw string) (string, error) {
	names, err := ParseHarnessList(raw)
	if err != nil {
		return "", err
	}
	if len(names) != 1 {
		return "", fmt.Errorf("--harness for list accepts one harness name, not a comma-separated list: %q", raw)
	}
	return names[0], nil
}
