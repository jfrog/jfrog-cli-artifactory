package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CrossAgentName is the reserved agent name for the shared cross-agent skills directory.
const CrossAgentName = ""

// AgentConfig holds the skills directory paths for an AI agent.
type AgentConfig struct {
	GlobalDir  string
	ProjectDir string
}

// Agents is the single source of truth for all supported AI agents and their skill paths.
// cross-agent is a special entry for the shared .agents/skills directory.
var Agents = map[string]AgentConfig{
	"claude-code":    {GlobalDir: "~/.claude/skills", ProjectDir: ".claude/skills"},
	"cursor":         {GlobalDir: "~/.cursor/skills-cursor", ProjectDir: ".cursor/skills"},
	"github-copilot": {GlobalDir: "~/.github/copilot/skills", ProjectDir: ".github/copilot/skills"},
	"windsurf":       {GlobalDir: "~/.windsurf/skills", ProjectDir: ".windsurf/skills"},
	CrossAgentName:   {GlobalDir: "~/.agents/skills", ProjectDir: ".agents/skills"},
}

// SupportedAgentsList returns a human-readable comma-separated list of agents
// that can be passed to --agent (excludes the internal cross-agent entry).
func SupportedAgentsList() string {
	names := make([]string, 0, len(Agents)-1)
	for name := range Agents {
		if name != CrossAgentName {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// ExpandHome expands a leading "~/" to the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// ResolveInstallPath determines the parent directory into which a skill should be installed.
//
//   - global=true  → expanded <agent.GlobalDir>
//   - default      → <projectDir>/<agent.ProjectDir>  (or <agent.ProjectDir> relative to CWD)
//
// Pass CrossAgentName as agentName to target the shared .agents/skills directory.
func ResolveInstallPath(agentName, projectDir string, global bool) (string, error) {
	agent, ok := Agents[strings.ToLower(agentName)]
	if !ok {
		return "", fmt.Errorf("unknown agent %q. Supported agents: %s", agentName, SupportedAgentsList())
	}

	if global {
		return ExpandHome(agent.GlobalDir), nil
	}

	if projectDir != "" {
		return filepath.Join(projectDir, agent.ProjectDir), nil
	}
	return agent.ProjectDir, nil
}
