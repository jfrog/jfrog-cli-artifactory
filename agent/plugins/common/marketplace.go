package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

// ErrMarketplaceNotFound is returned when <harness>-marketplace.json does not exist in the repo.
var ErrMarketplaceNotFound = errors.New("marketplace.json not found")

// InstallBypassMarketplaceHint explains how to install when marketplace lookup cannot resolve a version.
const InstallBypassMarketplaceHint = "re-run with --version <ver> (e.g. --version 1.0.0 or --version latest) " +
	"to install directly from Artifactory without using the marketplace"

// marketplaceEntry is a single plugin record in <harness>-marketplace.json.
// Unknown fields are ignored so the parser stays forward-compatible.
type marketplaceEntry struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Source  *pluginSource `json:"source,omitempty"`
}

// pluginSource describes where the plugin artifact is stored (only Source.URL is consumed).
type pluginSource struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	SHA    string `json:"sha"`
}

// marketplace is the on-disk shape of <harness>-marketplace.json.
type marketplace struct {
	Name    string             `json:"name"`
	Plugins []marketplaceEntry `json:"plugins"`
}

// MarketplaceFileName returns "<harness>-marketplace.json".
func MarketplaceFileName(harness string) string {
	return strings.ToLower(strings.TrimSpace(harness)) + "-marketplace.json"
}

// downloadMarketplace downloads <harness>-marketplace.json from the repository root into a temp dir.
// The returned cleanup function removes the temp dir; callers should defer it after consuming the file.
// When the marketplace.json is absent, the error is ErrMarketplaceNotFound (compare with errors.Is).
func downloadMarketplace(serverDetails *config.ServerDetails, repoKey, harness string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "plugin-marketplace-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create temp dir for marketplace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	serviceManager, err := utils.CreateDownloadServiceManager(serverDetails, agentcommon.PackageDownloadThreads, agentcommon.PackageDownloadRetries, 0, false, nil)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}

	fileName := MarketplaceFileName(harness)
	pattern := fmt.Sprintf("%s/%s", repoKey, fileName)

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = pattern
	downloadParams.Target = tmpDir + "/"
	downloadParams.Flat = true

	totalDownloaded, totalFailed, err := serviceManager.DownloadFiles(downloadParams)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("download %s failed: %w", fileName, err)
	}
	if totalDownloaded == 0 || totalFailed > 0 {
		cleanup()
		return "", func() {}, ErrMarketplaceNotFound
	}

	return filepath.Join(tmpDir, fileName), cleanup, nil
}

// parseMarketplace reads and parses a marketplace.json file at path.
func parseMarketplace(path string) (*marketplace, error) {
	// #nosec G304 -- path is the marketplace.json we just downloaded into our temp dir.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read marketplace %s: %w", path, err)
	}
	var parsed marketplace
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse marketplace %s: %w", path, err)
	}
	return &parsed, nil
}

// findEntry returns the marketplace entry for the given slug (case-insensitive match on Name).
func findEntry(marketplace *marketplace, slug string) (*marketplaceEntry, bool) {
	if marketplace == nil {
		return nil, false
	}
	target := strings.ToLower(strings.TrimSpace(slug))
	for index := range marketplace.Plugins {
		if strings.ToLower(strings.TrimSpace(marketplace.Plugins[index].Name)) == target {
			return &marketplace.Plugins[index], true
		}
	}
	return nil, false
}

// ResolveVersionFromMarketplace downloads <harness>-marketplace.json, looks up slug, and
// returns the version recorded there. The marketplace file is deleted before this function
// returns. Returns ErrMarketplaceNotFound (wrapped) when the marketplace file is missing.
func ResolveVersionFromMarketplace(serverDetails *config.ServerDetails, repoKey, harness, slug string) (string, error) {
	path, cleanup, err := downloadMarketplace(serverDetails, repoKey, harness)
	if err != nil {
		return "", err
	}
	defer cleanup()

	parsed, err := parseMarketplace(path)
	if err != nil {
		return "", err
	}
	entry, ok := findEntry(parsed, slug)
	if !ok {
		return "", fmt.Errorf("plugin '%s' is not listed in %s; %s", slug, MarketplaceFileName(harness), InstallBypassMarketplaceHint)
	}
	version := strings.TrimSpace(entry.Version)
	if version == "" {
		return "", fmt.Errorf("plugin '%s' in %s has no version; %s", slug, MarketplaceFileName(harness), InstallBypassMarketplaceHint)
	}
	return version, nil
}
