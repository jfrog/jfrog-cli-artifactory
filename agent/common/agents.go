package common

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ParseHarnessList parses comma-separated harness names: trims whitespace, lowercases,
// and rejects empty entries and duplicates.
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

// AgentConfig holds per-agent install directory paths for an AI harness.
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

// AgentRegistryHelpExample configures the agent-config.json snippet in registry help text.
type AgentRegistryHelpExample struct {
	ConfigSectionKey  string
	ExampleProjectDir string
	ExampleGlobalDir  string
}

// LoadAgentRegistry merges builtIns with the agent-config.json section keyed by configSectionKey.
// A missing file or section is not an error; built-in defaults are returned unchanged.
func LoadAgentRegistry(builtIns map[string]AgentConfig, configSectionKey string) (map[string]AgentSpec, error) {
	registry := make(map[string]AgentSpec, len(builtIns))
	for name, config := range builtIns {
		registry[strings.ToLower(name)] = AgentSpec{
			Name:       name,
			Config:     config,
			FromConfig: false,
		}
	}

	section, path, err := LoadAgentConfigSection(configSectionKey)
	if err != nil {
		return nil, err
	}
	if section == nil {
		return registry, nil
	}

	var agentsFromConfig map[string]AgentConfig
	if err := json.Unmarshal(section, &agentsFromConfig); err != nil {
		return nil, fmt.Errorf("failed to parse %q in %s: %w", configSectionKey, path, err)
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
func ResolveAgent(registry map[string]AgentSpec, name string, helpExample AgentRegistryHelpExample) (AgentSpec, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return AgentSpec{}, fmt.Errorf("agent name is required.\n%s", AgentRegistryHelp(registry, helpExample))
	}
	spec, ok := registry[normalizedName]
	if !ok {
		return AgentSpec{}, fmt.Errorf("unknown agent %q.\n%s", name, AgentRegistryHelp(registry, helpExample))
	}
	return spec, nil
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
func AgentRegistryHelp(registry map[string]AgentSpec, helpExample AgentRegistryHelpExample) string {
	configPath := AgentConfigPathForDisplay()
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
	fmt.Fprintf(&helpBuf, `  {
    %q: {
      "my-agent": { "projectDir": %q, "globalDir": %q }
    }
  }`, helpExample.ConfigSectionKey, helpExample.ExampleProjectDir, helpExample.ExampleGlobalDir)
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
func SupportedAgentsList(builtIns map[string]AgentConfig, configSectionKey string) string {
	registry, err := LoadAgentRegistry(builtIns, configSectionKey)
	if err != nil || len(registry) == 0 {
		names := make([]string, 0, len(builtIns))
		for name := range builtIns {
			names = append(names, name)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	return AgentNames(registry)
}
