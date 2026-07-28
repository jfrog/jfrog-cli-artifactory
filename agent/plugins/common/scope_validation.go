package common

import (
	"fmt"
	"strings"
)

// RejectUnsupportedProjectScope errors when isProjectScope is true and any agent is one of
// the hardcoded claude/cursor/codex, whose native plugin config is global-only. Custom
// agent-config.json agents aren't restricted. action (e.g. "install", "update") is used in
// the error message.
func RejectUnsupportedProjectScope(isProjectScope bool, agents []AgentSpec, action string) error {
	if !isProjectScope {
		return nil
	}
	for _, agent := range agents {
		switch strings.ToLower(agent.Name) {
		case "claude":
			return fmt.Errorf(
				"claude does not support project-scoped plugin %ss: "+
					"Claude plugin configuration is user-scoped only (~/.claude/settings.json). "+
					"Use --global instead", action,
			)
		case "cursor":
			return fmt.Errorf(
				"cursor does not support project-scoped plugin %ss: "+
					"Cursor only auto-discovers full plugins from ~/.cursor/plugins/local/. "+
					"Use --global instead", action,
			)
		case "codex":
			return fmt.Errorf(
				"codex does not support project-scoped plugin %ss: "+
					"Codex plugin configuration is user-scoped only (~/.codex/config.toml). "+
					"Use --global instead", action,
			)
		}
	}
	return nil
}
