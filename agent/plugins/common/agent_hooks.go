package common

import "strings"

// PostInstallFn runs after plugin files are copied to installDir for a fresh install.
// repoKey is used as the marketplace name so each repo gets its own isolated marketplace.
// If the native agent CLI is absent, it should log a warning and return nil.
type PostInstallFn func(slug, version, installDir, repoKey string) error

// PostUpdateFn runs after `jf agent plugins update` overwrites an already-installed
// plugin's files in place — refreshing an existing marketplace entry, not registering a
// new one. If the native agent CLI is absent, it should log a warning and return nil.
type PostUpdateFn func(slug, version, installDir, repoKey string) error

// agentPostInstallHooks maps lowercase agent names to their post-install hook.
// Agents without an entry have no native CLI integration and no hook runs.
var agentPostInstallHooks = map[string]PostInstallFn{
	"claude": claudePostInstall,
	"codex":  codexPostInstall,
}

// agentPostUpdateHooks maps lowercase agent names to their post-update hook.
// Agents without an entry have no native CLI integration; update is a pure
// file copy for them (e.g. cursor, or a direct --path install).
var agentPostUpdateHooks = map[string]PostUpdateFn{
	"claude": claudePostUpdate,
	"codex":  codexPostUpdate,
}

// RunPostInstallHook executes the registered post-install hook for agentName.
// Returns nil if no hook is registered.
func RunPostInstallHook(agentName, slug, version, installDir, repoKey string) error {
	fn, ok := agentPostInstallHooks[strings.ToLower(agentName)]
	if !ok {
		return nil
	}
	return fn(slug, version, installDir, repoKey)
}

// RunPostUpdateHook executes the registered post-update hook for agentName.
// Returns nil if no hook is registered.
func RunPostUpdateHook(agentName, slug, version, installDir, repoKey string) error {
	fn, ok := agentPostUpdateHooks[strings.ToLower(agentName)]
	if !ok {
		return nil
	}
	return fn(slug, version, installDir, repoKey)
}
