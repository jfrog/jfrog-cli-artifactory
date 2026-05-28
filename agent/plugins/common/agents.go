package common

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

// AgentConfig holds the plugin install directories for an AI agent.
type AgentConfig struct {
	GlobalDir  string `json:"globalDir"`
	ProjectDir string `json:"projectDir"`
}

// AgentSpec is a resolved agent; FromConfig marks JSON vs built-in.
type AgentSpec struct {
	Name       string
	Config     AgentConfig
	FromConfig bool
}

// Agents is the hardcoded set of agents currently supported by `jf agent plugins`.
// User overrides come from agent-config.json -> "plugins-agents".
var Agents = map[string]AgentConfig{
	"claude": {GlobalDir: "~/.claude/plugins", ProjectDir: ".claude/plugins"},
	"cursor": {GlobalDir: "~/.cursor/plugins", ProjectDir: ".cursor/plugins"},
	"codex":  {GlobalDir: "~/.codex/plugins", ProjectDir: ".codex/plugins"},
}

// LoadAgentRegistry merges the agent-config.json "plugins-agents" section over built-in
// Agents (keys lowercased). A missing file or section is not an error; built-in defaults
// are returned unchanged.
func LoadAgentRegistry() (map[string]AgentSpec, error) {
	registry := make(map[string]AgentSpec, len(Agents))
	for name, config := range Agents {
		registry[strings.ToLower(name)] = AgentSpec{
			Name:       name,
			Config:     config,
			FromConfig: false,
		}
	}

	section, path, err := agentcommon.LoadAgentConfigSection(agentcommon.PluginsAgentsKey)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return registry, nil
	}

	var agentsFromConfig map[string]AgentConfig
	if err := json.Unmarshal(section, &agentsFromConfig); err != nil {
		return nil, fmt.Errorf("failed to parse %q in %s: %w", agentcommon.PluginsAgentsKey, path, err)
	}

	for name, config := range agentsFromConfig {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if normalizedName == "" {
			return nil, fmt.Errorf("agent config %s contains an entry with an empty name", path)
		}
		if config.GlobalDir == "" && config.ProjectDir == "" {
			return nil, fmt.Errorf("agent %q in %s must define globalDir and/or projectDir", name, path)
		}
		registry[normalizedName] = AgentSpec{
			Name:       normalizedName,
			Config:     config,
			FromConfig: true,
		}
	}

	return registry, nil
}

// ResolveAgent returns spec or an error listing supported agents.
func ResolveAgent(registry map[string]AgentSpec, name string) (AgentSpec, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return AgentSpec{}, fmt.Errorf("agent name is required.\n%s", AgentRegistryHelp(registry))
	}
	spec, ok := registry[normalizedName]
	if !ok {
		return AgentSpec{}, fmt.Errorf("unknown agent %q.\n%s", name, AgentRegistryHelp(registry))
	}
	return spec, nil
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

// AgentNames returns the registry's agent names, sorted alphabetically.
func AgentNames(registry map[string]AgentSpec) string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// AgentRegistryHelp lists agents/paths and how to edit agent-config.json.
func AgentRegistryHelp(registry map[string]AgentSpec) string {
	configPath, _ := agentcommon.AgentConfigPath()
	keys := make([]string, 0, len(registry))
	for name := range registry {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	var helpBuf strings.Builder
	helpBuf.WriteString("Supported agents:\n")
	for _, name := range keys {
		spec := registry[name]
		fmt.Fprintf(&helpBuf, "  - %s (project: %s, global: %s)\n", name, spec.Config.ProjectDir, spec.Config.GlobalDir)
	}
	fmt.Fprintf(&helpBuf, "\nTo add or override an agent, edit %s. Example:\n", configPath)
	helpBuf.WriteString(`  {
    "plugins-agents": {
      "my-agent": { "projectDir": ".my-agent/plugins", "globalDir": "~/.my-agent/plugins" }
    }
  }`)
	return helpBuf.String()
}

// ResolveAgentInstallDir is absolute global dir or projectDir + project-relative path.
func ResolveAgentInstallDir(spec AgentSpec, projectDir string, global bool) (string, error) {
	if global {
		dir := agentcommon.ExpandHome(spec.Config.GlobalDir)
		if dir == "" {
			return "", fmt.Errorf("agent %q has no global directory configured", spec.Name)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("invalid global path for agent %q: %w", spec.Name, err)
		}
		return abs, nil
	}
	if spec.Config.ProjectDir == "" {
		return "", fmt.Errorf("agent %q has no project directory configured", spec.Name)
	}
	if projectDir == "" {
		projectDir = "."
	}
	return filepath.Abs(filepath.Join(projectDir, spec.Config.ProjectDir))
}

// SupportedAgentsList is comma-separated names from registry or built-ins.
func SupportedAgentsList() string {
	registry, err := LoadAgentRegistry()
	if err != nil || len(registry) == 0 {
		names := make([]string, 0, len(Agents))
		for name := range Agents {
			names = append(names, name)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	return AgentNames(registry)
}
