package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// Keys recognized inside ~/.jfrog/agents/agent-config.json.
const (
	agentsConfigSubdir = "agents"
	agentsConfigFile   = "agent-config.json"

	// SkillsAgentsKey holds the per-agent skills directory overrides.
	SkillsAgentsKey = "skills-agents"
	// PluginsAgentsKey holds the per-agent plugins directory overrides.
	PluginsAgentsKey = "plugins-agents"
	// PluginManifestPathsKey holds the ordered list of relative plugin.json paths checked by publish.
	PluginManifestPathsKey = "plugin-manifest-paths"
)

// AgentConfigPath returns ~/.jfrog/agents/agent-config.json. The file may not exist.
func AgentConfigPath() (string, error) {
	home, err := coreutils.GetJfrogHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, agentsConfigSubdir, agentsConfigFile), nil
}

// LoadAgentConfigSection reads ~/.jfrog/agents/agent-config.json and returns the
// raw JSON for the requested top-level key. When the file or the key is missing
// it returns (nil, path, nil). The resolved path is always returned so callers
// can include it in error messages.
func LoadAgentConfigSection(key string) (json.RawMessage, string, error) {
	path, err := AgentConfigPath()
	if err != nil {
		return nil, "", fmt.Errorf("resolve agent config path: %w", err)
	}

	// #nosec G304 -- path is constructed from the JFrog home dir, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, path, nil
		}
		return nil, path, fmt.Errorf("failed to read agent config %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, path, nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, path, fmt.Errorf("failed to parse agent config %s: %w", path, err)
	}
	section, ok := doc[key]
	if !ok || len(section) == 0 {
		return nil, path, nil
	}
	return section, path, nil
}
