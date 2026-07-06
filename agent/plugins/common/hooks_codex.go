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

// CodexExec dispatches native codex CLI commands.
// Exported so that tests in other packages can swap it with a no-op.
var CodexExec = func(args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), codexNativeCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "codex", args...).CombinedOutput() // #nosec G204 -- args are tool-managed subcommand strings; slug is pre-validated by ValidateSlug
	if err != nil {
		log.Warn("codex " + strings.Join(args, " ") + ": " + string(out))
	}
}

// codexPostInstall writes the plugin into the Codex marketplace manifest and
// registers it with the native codex CLI (if available).
//
// Directory layout produced by agents.go (GlobalDir = ~/.agents/plugins/local/<repoKey>):
//
//	~/.agents/plugins/local/<repoKey>/          ← marketplace root
//	  .agents/plugins/
//	    marketplace.json                       ← written here
//	  <slug>/                                  ← installDir (plugin files copied by jf)
func codexPostInstall(slug, version, installDir, repoKey string) error {
	manifestPath := codexMarketplaceManifestPath(installDir)
	log.Info(fmt.Sprintf("[codex] writing marketplace entry for '%s' → %s", slug, manifestPath))
	if err := upsertCodexMarketplaceEntry(manifestPath, slug, repoKey); err != nil {
		return err
	}
	_, err := lookPathCodex()
	if err == nil {
		// CLI found, proceed with registration
		root := codexMarketplaceRoot(installDir)
		log.Info(fmt.Sprintf("[codex] registering marketplace: codex plugin marketplace add %s", root))
		CodexExec("plugin", "marketplace", "add", root)
		log.Info(fmt.Sprintf("[codex] installing plugin: codex plugin add %s@%s", slug, repoKey))
		// Include the @<repoKey> qualifier so Codex resolves the correct marketplace source.
		CodexExec("plugin", "add", slug+"@"+repoKey)
	} else {
		// CLI not found, log warning but continue (not a fatal error)
		log.Warn("[codex] codex CLI not found on PATH; skipping native marketplace registration. " +
			"Run: codex plugin marketplace add " + codexMarketplaceRoot(installDir))
	}
	return nil
}

// codexMarketplaceRoot returns the marketplace root directory.
//
//	installDir = ~/.agents/plugins/local/<repoKey>/<slug>
//	root       = ~/.agents/plugins/local/<repoKey>
func codexMarketplaceRoot(installDir string) string {
	return filepath.Dir(installDir)
}

// codexMarketplaceManifestPath returns the path to the Codex marketplace manifest.
//
//	installDir = ~/.agents/plugins/local/<repoKey>/<slug>
//	manifest   = ~/.agents/plugins/local/<repoKey>/.agents/plugins/marketplace.json
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
	for i, p := range m.Plugins {
		if strings.EqualFold(p.Name, slug) {
			m.Plugins[i] = entry
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

// lookPathCodex is a variable so tests can override it without hitting the real PATH.
var lookPathCodex = func() (string, error) {
	return exec.LookPath("codex")
}
