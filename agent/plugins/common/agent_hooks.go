package common

import "strings"

// PostInstallFn is called after plugin files are copied to installDir.
// repoKey is the Artifactory repository the plugin was installed from and is
// used as the marketplace name so each repo gets its own isolated marketplace.
// If the native agent CLI is absent the function should log a warning and
// return nil — the file installation already succeeded.
type PostInstallFn func(slug, version, installDir, repoKey string) error

// agentPostInstallHooks maps lowercase agent names to their post-install hook.
// Agents without an entry have no native CLI integration and no hook runs.
var agentPostInstallHooks = map[string]PostInstallFn{
	"claude": claudePostInstall,
	"codex":  codexPostInstall,
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
