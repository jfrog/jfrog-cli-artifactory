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

// AgentConfig holds the install directory paths for an AI agent (project and global).
type AgentConfig struct {
	GlobalDir  string `json:"globalDir"`
	ProjectDir string `json:"projectDir"`
}

// AgentSpec is a resolved agent. FromConfig marks JSON-overridden vs built-in entries.
type AgentSpec struct {
	Name       string
	Config     AgentConfig
	FromConfig bool
}

// AgentPackageConfig describes the package-type-specific bits of the agent registry.
// Skills and plugins share the same shape but use different config files and defaults.
type AgentPackageConfig struct {
	// ConfigFileName is the JSON file name under ~/.jfrog/agents/ (e.g. "agent-plugin-config.json").
	ConfigFileName string
	// Defaults are the built-in agent entries merged under the JSON overrides.
	Defaults map[string]AgentConfig
}

const agentsConfigSubdir = "agents"

type agentConfigDocument struct {
	Agents map[string]AgentConfig `json:"agents"`
}

// AgentConfigPath returns ~/.jfrog/agents/<configFileName>. The file may be missing.
func AgentConfigPath(packageConfig AgentPackageConfig) (string, error) {
	home, err := coreutils.GetJfrogHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, agentsConfigSubdir, packageConfig.ConfigFileName), nil
}

// LoadAgentRegistry merges <configFileName> over the built-in defaults (keys lowercased).
// A missing config file is not an error; built-in defaults are returned unchanged.
func LoadAgentRegistry(packageConfig AgentPackageConfig) (map[string]AgentSpec, error) {
	registry := make(map[string]AgentSpec, len(packageConfig.Defaults))
	for name, config := range packageConfig.Defaults {
		registry[strings.ToLower(name)] = AgentSpec{
			Name:       name,
			Config:     config,
			FromConfig: false,
		}
	}

	path, err := AgentConfigPath(packageConfig)
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

// ResolveAgent returns the spec for name, or an error listing supported agents.
func ResolveAgent(registry map[string]AgentSpec, name string) (AgentSpec, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return AgentSpec{}, fmt.Errorf("agent name is required.\n%s", AgentRegistryHelp(registry, ""))
	}
	spec, ok := registry[normalizedName]
	if !ok {
		return AgentSpec{}, fmt.Errorf("unknown agent %q.\n%s", name, AgentRegistryHelp(registry, ""))
	}
	return spec, nil
}

// ParseAgentList parses comma-separated agent names (trim, lowercase, reject empty/duplicates).
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

// AgentRegistryHelp lists agents/paths and points to the JSON config file at configPath
// when non-empty. When configPath is empty, only the supported list is rendered.
func AgentRegistryHelp(registry map[string]AgentSpec, configPath string) string {
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
	if configPath != "" {
		fmt.Fprintf(&helpBuf, "\nTo add or override an agent, edit %s.", configPath)
	}
	return helpBuf.String()
}

// ResolveAgentInstallDir returns the absolute global dir or projectDir joined with projectRoot.
func ResolveAgentInstallDir(spec AgentSpec, projectRoot string, global bool) (string, error) {
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
	if projectRoot == "" {
		projectRoot = "."
	}
	return filepath.Abs(filepath.Join(projectRoot, spec.Config.ProjectDir))
}

// SupportedAgentsList returns the comma-separated agent names for the given package type.
// On registry errors it falls back to the built-in defaults.
func SupportedAgentsList(packageConfig AgentPackageConfig) string {
	registry, err := LoadAgentRegistry(packageConfig)
	if err != nil || len(registry) == 0 {
		names := make([]string, 0, len(packageConfig.Defaults))
		for name := range packageConfig.Defaults {
			names = append(names, name)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	return AgentNames(registry)
}

// ExpandHome maps a leading "~/" to the user's home directory.
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
