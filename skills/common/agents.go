package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// AgentConfig holds the skills directory paths for an AI agent.
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

const (
	agentsConfigSubdir = "agents"
	agentsConfigFile   = "agent-config.json"
)

type agentConfigDocument struct {
	Agents map[string]AgentConfig `json:"agents"`
}

// AgentConfigPath is ~/.jfrog/agents/agent-config.json (file may be missing).
func AgentConfigPath() (string, error) {
	home, err := coreutils.GetJfrogHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, agentsConfigSubdir, agentsConfigFile), nil
}

// LoadAgentRegistry merges agent-config.json over built-in Agents (keys lowercased).
// A missing agent-config.json is not an error; built-in defaults are returned unchanged.
func LoadAgentRegistry() (map[string]AgentSpec, error) {
	registry := make(map[string]AgentSpec, len(Agents))
	for name, config := range Agents {
		registry[strings.ToLower(name)] = AgentSpec{
			Name:       name,
			Config:     config,
			FromConfig: false,
		}
	}

	path, err := AgentConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve agent config path: %w", err)
	}

	// #nosec G304 -- path is constructed from the JFrog home dir, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return registry, nil
		}
		return nil, fmt.Errorf("failed to read agent config %s: %w", path, err)
	}

	if len(data) == 0 {
		return registry, nil
	}

	var parsedConfig agentConfigDocument
	if err := json.Unmarshal(data, &parsedConfig); err != nil {
		return nil, fmt.Errorf("failed to parse agent config %s: %w", path, err)
	}

	for name, config := range parsedConfig.Agents {
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

// ParseAgentList parses comma-separated agent names (trim, lower, no dups).
func ParseAgentList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("--agent is required (comma-separated list of agent names)")
	}

	seen := make(map[string]struct{})
	var result []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			return nil, fmt.Errorf("--agent contains an empty name in %q", raw)
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("--agent lists %q more than once", name)
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result, nil
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
	configPath, _ := AgentConfigPath()
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
    "agents": {
      "my-agent": { "projectDir": ".my-agent/skills", "globalDir": "~/.my-agent/skills" }
    }
  }`)
	return helpBuf.String()
}

// ResolveAgentInstallDir is absolute global dir or projectDir + project-relative path.
func ResolveAgentInstallDir(spec AgentSpec, projectDir string, global bool) (string, error) {
	if global {
		dir := ExpandHome(spec.Config.GlobalDir)
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
