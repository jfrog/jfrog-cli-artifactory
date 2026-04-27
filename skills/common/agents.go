package common

import "strings"

// AgentConfig holds the skills directory paths for an AI agent.
type AgentConfig struct {
	GlobalDir  string // e.g. ~/.cursor/skills
	ProjectDir string // e.g. .cursor/skills (relative to project root)
}

// Agents is the single source of truth for all supported AI agents and their skill paths.
var Agents = map[string]AgentConfig{
	"claude-code":    {GlobalDir: "~/.claude/skills", ProjectDir: ".claude/skills"},
	"cursor":         {GlobalDir: "~/.cursor/skills", ProjectDir: ".cursor/skills"},
	"github-copilot": {GlobalDir: "~/.github/copilot/skills", ProjectDir: ".github/copilot/skills"},
	"windsurf":       {GlobalDir: "~/.windsurf/skills", ProjectDir: ".windsurf/skills"},
}

// SupportedAgentsList returns a human-readable comma-separated list of supported agent names.
func SupportedAgentsList() string {
	names := make([]string, 0, len(Agents))
	for name := range Agents {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
