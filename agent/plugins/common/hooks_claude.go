package common

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

const claudeNativeCmdTimeout = 30 * time.Second

// claudePostInstall writes the plugin into the JFrog marketplace file and
// registers it with the native claude CLI (if available).
//
// Directory layout produced by agents.go (GlobalDir = ~/.claude/plugins/local/jfrog-plugins):
//
//	~/.claude/plugins/local/jfrog-plugins/
//	  .claude-plugin/
//	    marketplace.json          ← written here
//	  <slug>/                     ← installDir (plugin files copied by jf)
func claudePostInstall(slug, version, installDir, repoKey string) error {
	marketplacePath := claudeMarketplacePath(installDir)
	log.Info(fmt.Sprintf("[claude] writing marketplace entry for '%s' → %s", slug, marketplacePath))
	if err := upsertLocalMarketplaceEntry(marketplacePath, slug, version, repoKey); err != nil {
		return err
	}
	if _, err := lookPathClaude(); err != nil {
		log.Warn("[claude] claude CLI not found on PATH; skipping native marketplace registration. " +
			"Install the Claude CLI to complete native plugin registration.")
		return nil
	}
	// Pass the marketplace root directory, not the .json file itself.
	// `claude plugin marketplace add <dir>` reads <dir>/.claude-plugin/marketplace.json.
	marketplaceDir := claudeMarketplaceDir(installDir)
	log.Info(fmt.Sprintf("[claude] registering marketplace: claude plugin marketplace add %s", marketplaceDir))
	ClaudeExec("plugin", "marketplace", "add", marketplaceDir)
	log.Info(fmt.Sprintf("[claude] installing plugin: claude plugin install %s@%s", slug, repoKey))
	// Include the @<repoKey> qualifier so Claude resolves the correct marketplace source.
	ClaudeExec("plugin", "install", slug+"@"+repoKey)
	return nil
}

// claudePostDelete removes the plugin from the JFrog marketplace file and
// unregisters it from the native claude CLI (if available).
func claudePostDelete(slug, installDir, repoKey string) error {
	marketplacePath := claudeMarketplacePath(installDir)
	log.Info(fmt.Sprintf("[claude] removing marketplace entry for '%s' from %s", slug, marketplacePath))
	if err := removeLocalMarketplaceEntry(marketplacePath, slug); err != nil {
		return err
	}
	if _, err := lookPathClaude(); err != nil {
		return nil
	}
	log.Info(fmt.Sprintf("[claude] uninstalling plugin: claude plugin uninstall %s@%s", slug, repoKey))
	ClaudeExec("plugin", "uninstall", slug+"@"+repoKey)
	return nil
}

// claudeMarketplacePath returns the path to the JFrog marketplace file inside
// the marketplace root directory.
//
	//	installDir  = ~/.claude/plugins/local/jfrog-plugins/<slug>
	//	marketplace = ~/.claude/plugins/local/jfrog-plugins/.claude-plugin/marketplace.json
func claudeMarketplacePath(installDir string) string {
	return filepath.Join(claudeMarketplaceDir(installDir), ".claude-plugin", "marketplace.json")
}

// claudeMarketplaceDir returns the marketplace root (the directory that contains
// .claude-plugin/marketplace.json and all installed plugin subdirectories).
func claudeMarketplaceDir(installDir string) string {
	return filepath.Dir(installDir)
}

// lookPathClaude is a variable so tests can override it without hitting the real PATH.
var lookPathClaude = func() (string, error) {
	return exec.LookPath("claude")
}

// ClaudeExec is the function used to dispatch native claude CLI commands.
// It is exported so that tests in other packages can swap it with a no-op to
// avoid invoking the real claude binary (which would touch user state and emit warnings).
var ClaudeExec = func(args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), claudeNativeCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "claude", args...).CombinedOutput() // #nosec G204 -- args are tool-managed subcommand strings; slug is pre-validated by ValidateSlug
	if err != nil {
		log.Warn("claude " + strings.Join(args, " ") + ": " + string(out))
	}
}
