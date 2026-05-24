package common

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

// pluginConfigDocument is the on-disk shape of agent-plugin-config.json.
// "agents" is consumed by LoadAgentRegistry; "manifestPaths" extends the
// hardcoded plugin.json search list used by publish.
type pluginConfigDocument struct {
	Agents        map[string]agentcommon.AgentConfig `json:"agents"`
	ManifestPaths []string                           `json:"manifestPaths"`
}

// ResolveManifestSearchPaths returns the ordered list of relative paths to consult when
// looking for plugin.json. Hardcoded KnownManifestRelPaths come first (highest priority);
// any extra entries from agent-plugin-config.json are appended (deduplicated).
// A missing or malformed config file falls back to the hardcoded defaults only.
func ResolveManifestSearchPaths() []string {
	merged := append([]string(nil), KnownManifestRelPaths...)
	seen := make(map[string]struct{}, len(merged))
	for _, path := range merged {
		seen[path] = struct{}{}
	}

	extras, err := loadExtraManifestPaths()
	if err != nil {
		return merged
	}
	for _, path := range extras {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}
	return merged
}

func loadExtraManifestPaths() ([]string, error) {
	configPath, err := agentcommon.AgentConfigPath(PackageConfig())
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- path is constructed from the JFrog home dir, not user input.
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var parsedConfig pluginConfigDocument
	if err := json.Unmarshal(data, &parsedConfig); err != nil {
		return nil, err
	}
	return parsedConfig.ManifestPaths, nil
}
