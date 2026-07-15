package common

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const claudeNativeCmdTimeout = 30 * time.Second

// claudePostInstall writes the plugin into the JFrog marketplace file and
// registers it with the native claude CLI (if available).
// Returns an error if the marketplace write fails or if native registration fails when the CLI is found.
// Returns a WarningError if the CLI is not found on PATH.
//
// Directory layout produced by agents.go (GlobalDir = ~/.claude/plugins/local/jfrog):
//
//	~/.claude/plugins/local/jfrog/
//	  .claude-plugin/
//	    marketplace.json          ← written here
//	  <slug>/                     ← installDir (plugin files copied by jf)
func claudePostInstall(slug, version, installDir, repoKey string) error {
	marketplacePath := claudeMarketplacePath(installDir)
	log.Info(fmt.Sprintf("[claude] writing marketplace entry for '%s' → %s", slug, marketplacePath))
	if err := upsertLocalMarketplaceEntry(marketplacePath, slug, version, repoKey); err != nil {
		return err
	}
	marketplaceDir := claudeMarketplaceDir(installDir)
	_, err := LookPathClaude()
	if err == nil {
		// CLI found, proceed with registration
		log.Info(fmt.Sprintf("[claude] registering marketplace: claude plugin marketplace add %s", marketplaceDir))
		if execErr := ClaudeExec("plugin", "marketplace", "add", marketplaceDir); execErr != nil {
			return fmt.Errorf("native marketplace registration failed: %w", execErr)
		}
		log.Info(fmt.Sprintf("[claude] installing plugin: claude plugin install %s@%s", slug, repoKey))
		// Include the @<repoKey> qualifier so Claude resolves the correct marketplace source.
		if execErr := ClaudeExec("plugin", "install", slug+"@"+repoKey); execErr != nil {
			return fmt.Errorf("native plugin installation failed: %w", execErr)
		}
	} else {
		// CLI not found, return a warning so it's reported as a warning not a hard failure
		return agentcommon.NewWarningError(
			fmt.Sprintf("claude CLI not found on PATH; skipping native marketplace registration. "+
				"Install the Claude CLI to complete native plugin registration at %s", marketplaceDir),
		)
	}
	return nil
}

// claudeMarketplacePath returns the path to the JFrog marketplace file inside
// the marketplace root directory.
//
//	installDir  = ~/.claude/plugins/local/jfrog/<slug>
//	marketplace = ~/.claude/plugins/local/jfrog/.claude-plugin/marketplace.json
func claudeMarketplacePath(installDir string) string {
	return filepath.Join(claudeMarketplaceDir(installDir), ".claude-plugin", "marketplace.json")
}

// claudeMarketplaceDir returns the marketplace root (the directory that contains
// .claude-plugin/marketplace.json and all installed plugin subdirectories).
func claudeMarketplaceDir(installDir string) string {
	return filepath.Dir(installDir)
}

// LookPathClaude is a variable so tests can override it without hitting the real PATH.
// Exported so that tests in other packages can swap it to avoid depending on
// whether the claude CLI happens to be installed on the machine running the tests.
var LookPathClaude = func() (string, error) {
	return exec.LookPath("claude")
}

// ClaudeExec is the function used to dispatch native claude CLI commands and returns an error if the command fails.
// It is exported so that tests in other packages can swap it with a no-op to
// avoid invoking the real claude binary (which would touch user state and emit warnings).
var ClaudeExec = func(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), claudeNativeCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "claude", args...).CombinedOutput() // #nosec G204 -- args are tool-managed subcommand strings; slug is pre-validated by ValidateSlug
	if err != nil {
		return fmt.Errorf("claude %s: %s", strings.Join(args, " "), string(out))
	}
	return nil
}
