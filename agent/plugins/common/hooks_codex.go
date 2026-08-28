package common

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const codexNativeCmdTimeout = 30 * time.Second

// codexMarketplace is the on-disk shape of <marketplace-root>/.agents/plugins/marketplace.json.
// This is the "supported manifest" that `codex plugin marketplace add <root>` reads.
type codexMarketplace struct {
	Name      string             `json:"name"`
	Interface codexDisplayName   `json:"interface,omitempty"`
	Plugins   []codexPluginEntry `json:"plugins"`
}

type codexDisplayName struct {
	DisplayName string `json:"displayName,omitempty"`
}

// codexPluginEntry is a single plugin record inside the Codex marketplace manifest.
type codexPluginEntry struct {
	Name   string         `json:"name"`
	Source codexPluginSrc `json:"source"`
	Policy codexPolicy    `json:"policy,omitempty"`
}

// codexPluginSrc uses the object form that Codex requires.
// path is relative to the marketplace root (e.g. "./plugins/my-plugin").
type codexPluginSrc struct {
	Source string `json:"source"` // always "local"
	Path   string `json:"path"`   // "./plugins/<slug>"
}

type codexPolicy struct {
	Installation string `json:"installation,omitempty"`
}

// CodexExec dispatches native codex CLI commands and returns an error if the command fails.
// Exported so that tests in other packages can swap it with a no-op.
var CodexExec = func(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), codexNativeCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "codex", args...).CombinedOutput() // #nosec G204 -- args are tool-managed subcommand strings; slug is pre-validated by ValidateSlug
	if err != nil {
		return fmt.Errorf("codex %s: %s", strings.Join(args, " "), string(out))
	}
	return nil
}

// codexPostInstall writes the plugin into the Codex marketplace manifest (under
// installDir's parent, GlobalDir = ~/.agents/plugins/local/jfrog) and registers it with
// the native codex CLI if available. Returns an error on write/registration failure, or a
// WarningError if the CLI isn't on PATH.
func codexPostInstall(slug, version, installDir, repoKey string) error {
	manifestPath := codexMarketplaceManifestPath(installDir)
	log.Info(fmt.Sprintf("[codex] writing marketplace entry for '%s' → %s", slug, manifestPath))
	if err := upsertCodexMarketplaceEntry(manifestPath, slug, repoKey); err != nil {
		return err
	}
	_, err := LookPathCodex()
	if err == nil {
		// CLI found, proceed with registration
		root := codexMarketplaceRoot(installDir)
		log.Info(fmt.Sprintf("[codex] registering marketplace: codex plugin marketplace add %s", root))
		if execErr := CodexExec("plugin", "marketplace", "add", root); execErr != nil {
			return fmt.Errorf("native marketplace registration failed: %w", execErr)
		}
		log.Info(fmt.Sprintf("[codex] installing plugin: codex plugin add %s@%s", slug, repoKey))
		// Include the @<repoKey> qualifier so Codex resolves the correct marketplace source.
		if execErr := CodexExec("plugin", "add", slug+"@"+repoKey); execErr != nil {
			return fmt.Errorf("native plugin installation failed: %w", execErr)
		}
	} else {
		// CLI not found, return a warning so it's reported as a warning not a hard failure
		return agentcommon.NewWarningError(
			fmt.Sprintf("codex CLI not found on PATH; skipping native marketplace registration. "+
				"Run: codex plugin marketplace add %s", codexMarketplaceRoot(installDir)),
		)
	}
	return nil
}

// codexPostUpdate refreshes the marketplace manifest entry to the new version and asks
// the native codex CLI to pick it up. Unlike Claude, Codex has no plugin-level "update"
// verb (plugin add is also the resync verb) — only `marketplace upgrade` to refresh the
// catalog, so that's used for the marketplace step instead of re-`add`ing it.
func codexPostUpdate(slug, version, installDir, repoKey string) error {
	manifestPath := codexMarketplaceManifestPath(installDir)
	log.Info(fmt.Sprintf("[codex] refreshing marketplace entry for '%s' -> %s (v%s)", slug, manifestPath, version))
	if err := upsertCodexMarketplaceEntry(manifestPath, slug, repoKey); err != nil {
		return err
	}
	_, err := LookPathCodex()
	if err != nil {
		// CLI not found, log warning but continue (not a fatal error)
		log.Warn("[codex] codex CLI not found on PATH; skipping native marketplace refresh. " +
			"Run: codex plugin marketplace add " + codexMarketplaceRoot(installDir))
		return nil //nolint:nilerr // deliberate: missing CLI is non-fatal, not an error to propagate
	}
	// CLI found, proceed with marketplace refresh + plugin resync.
	log.Info(fmt.Sprintf("[codex] refreshing marketplace: codex plugin marketplace upgrade %s", repoKey))
	if execErr := CodexExec("plugin", "marketplace", "upgrade", repoKey); execErr != nil {
		// Cosmetic: only refreshes the catalog view (and always fails for jf's local
		// marketplaces — codex's upgrade verb expects a Git marketplace). The plugin
		// resync call below is what actually matters for native state.
		log.Warn(fmt.Sprintf("[codex] marketplace refresh failed: %v", execErr))
	}
	log.Info(fmt.Sprintf("[codex] updating plugin: codex plugin add %s@%s", slug, repoKey))
	if execErr := CodexExec("plugin", "add", slug+"@"+repoKey); execErr != nil {
		// Plugin files on disk are already updated; only native re-registration failed.
		// Surface as a warning (not swallowed) so the row doesn't misreport "ok" while
		// codex's own registry is left stale/unregistered.
		return agentcommon.NewWarningError(fmt.Sprintf("native plugin resync failed: %v", execErr))
	}
	return nil
}

// codexMarketplaceRoot returns the marketplace root directory.
//
//	installDir = ~/.agents/plugins/local/jfrog/<slug>
//	root       = ~/.agents/plugins/local/jfrog
func codexMarketplaceRoot(installDir string) string {
	return filepath.Dir(installDir)
}

// codexMarketplaceManifestPath returns the path to the Codex marketplace manifest.
//
//	installDir = ~/.agents/plugins/local/jfrog/<slug>
//	manifest   = ~/.agents/plugins/local/jfrog/.agents/plugins/marketplace.json
func codexMarketplaceManifestPath(installDir string) string {
	return filepath.Join(codexMarketplaceRoot(installDir), ".agents", "plugins", "marketplace.json")
}

// upsertCodexMarketplaceEntry reads or creates the Codex marketplace manifest at path,
// adds or replaces the entry for slug, then writes it back.
// marketplaceName is set as the manifest's "name" field (typically the Artifactory repo key).
func upsertCodexMarketplaceEntry(path, slug, marketplaceName string) error {
	m, err := readOrCreateCodexMarketplace(path, marketplaceName)
	if err != nil {
		return err
	}
	entry := codexPluginEntry{
		Name:   slug,
		Source: codexPluginSrc{Source: "local", Path: "./" + slug},
		Policy: codexPolicy{Installation: "AVAILABLE"},
	}
	found := false
	for index, plugin := range m.Plugins {
		if strings.EqualFold(plugin.Name, slug) {
			m.Plugins[index] = entry
			found = true
			break
		}
	}
	if !found {
		m.Plugins = append(m.Plugins, entry)
	}
	return writeCodexMarketplace(path, m)
}

// removeCodexMarketplaceEntry removes the slug entry from the Codex marketplace manifest.
// A missing file or missing slug entry is a no-op.
func removeCodexMarketplaceEntry(path, slug string) error {
	m, err := readOrCreateCodexMarketplace(path, "")
	if err != nil {
		return err
	}
	n := len(m.Plugins)
	kept := m.Plugins[:0]
	for _, p := range m.Plugins {
		if !strings.EqualFold(p.Name, slug) {
			kept = append(kept, p)
		}
	}
	if len(kept) == n {
		return nil
	}
	m.Plugins = kept
	return writeCodexMarketplace(path, m)
}

// readOrCreateCodexMarketplace reads and parses path. When the file does not exist,
// it returns an empty marketplace with the given name.
func readOrCreateCodexMarketplace(path, marketplaceName string) (*codexMarketplace, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is tool-managed config under agent home
	if err != nil {
		if os.IsNotExist(err) {
			return &codexMarketplace{
				Name:      marketplaceName,
				Interface: codexDisplayName{DisplayName: "JFrog Plugins"},
				Plugins:   []codexPluginEntry{},
			}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m codexMarketplace
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Plugins == nil {
		m.Plugins = []codexPluginEntry{}
	}
	return &m, nil
}

// writeCodexMarketplace creates parent directories as needed and writes m to path.
func writeCodexMarketplace(path string, m *codexMarketplace) error {
	if err := os.MkdirAll(filepath.Dir(path), agentcommon.InstallDirMode); err != nil {
		return fmt.Errorf("create dirs for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal codex marketplace: %w", err)
	}
	if err := os.WriteFile(path, data, agentcommon.InstallManifestFileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LookPathCodex is a variable so tests can override it without hitting the real PATH.
// Exported so that tests in other packages can swap it to avoid depending on
// whether the codex CLI happens to be installed on the machine running the tests.
var LookPathCodex = func() (string, error) {
	return exec.LookPath("codex")
}
