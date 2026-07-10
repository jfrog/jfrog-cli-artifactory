package common

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const nativeListCmdTimeout = 30 * time.Second

// IsRegisteredWithNativeAgent reports whether "<slug>@<repoKey>" is still registered in
// the native agent's own state — a native `claude plugin uninstall`/`codex plugin remove`
// clears that without touching jf's .jfrog/plugin-info.json, so jf's own bookkeeping alone
// can't detect it. Agents with no native registry (e.g. cursor, --path) always return
// (true, nil). Shells out to `plugin list --json` rather than parsing internal state files
// (undocumented, can go stale); a non-nil error means "couldn't verify", not "not registered".
func IsRegisteredWithNativeAgent(agentName, slug, repoKey string) (bool, error) {
	switch strings.ToLower(agentName) {
	case "claude":
		return isRegisteredWithClaude(slug, repoKey)
	case "codex":
		return isRegisteredWithCodex(slug, repoKey)
	default:
		return true, nil
	}
}

// HasNativeRegistry reports whether agentName has a native registry that
// IsRegisteredWithNativeAgent actually queries. Agents without one always get an
// unconditional (true, nil) from that function — meaning "no check available", not
// "confirmed present" — so callers needing to tell those apart must gate on this first.
func HasNativeRegistry(agentName string) bool {
	switch strings.ToLower(agentName) {
	case "claude", "codex":
		return true
	default:
		return false
	}
}

// claudePluginListJSON runs `claude plugin list --json` and returns its stdout.
// Overridable in tests. Queries Claude's live view rather than parsing
// installed_plugins.json, which is undocumented and can carry stale entries.
var claudePluginListJSON = func() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nativeListCmdTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "claude", "plugin", "list", "--json").Output() // #nosec G204 -- fixed subcommand, no user input
}

// isRegisteredWithClaude asks `claude plugin list --json` for a
// "<slug>@<repoKey>" entry, e.g.:
//
//	[{"id": "jfrog-plugin-timepass@buk-plugins-2", "version": "1.0.1", "enabled": true, ...}]
func isRegisteredWithClaude(slug, repoKey string) (bool, error) {
	out, err := claudePluginListJSON()
	if err != nil {
		return false, fmt.Errorf("claude plugin list --json: %w", err)
	}
	var plugins []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &plugins); err != nil {
		return false, fmt.Errorf("parse claude plugin list --json output: %w", err)
	}
	want := slug + "@" + repoKey
	for _, p := range plugins {
		if p.ID == want {
			return true, nil
		}
	}
	return false, nil
}

// codexPluginListJSON runs `codex plugin list --json` and returns its stdout.
// Overridable in tests. Queries Codex's live cache rather than config.toml, whose
// [plugins."..."] tables can persist even after the plugin's marketplace is gone.
var codexPluginListJSON = func() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), nativeListCmdTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "codex", "plugin", "list", "--json").Output() // #nosec G204 -- fixed subcommand, no user input
}

// isRegisteredWithCodex asks `codex plugin list --json` for a
// "<slug>@<repoKey>" entry in its "installed" array, e.g.:
//
//	{"installed": [{"pluginId": "jfrog-plugin-timepass@buk-plugins-2", "version": "1.0.1", ...}], "available": [...]}
func isRegisteredWithCodex(slug, repoKey string) (bool, error) {
	out, err := codexPluginListJSON()
	if err != nil {
		return false, fmt.Errorf("codex plugin list --json: %w", err)
	}
	var result struct {
		Installed []struct {
			PluginID string `json:"pluginId"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return false, fmt.Errorf("parse codex plugin list --json output: %w", err)
	}
	want := slug + "@" + repoKey
	for _, p := range result.Installed {
		if p.PluginID == want {
			return true, nil
		}
	}
	return false, nil
}
