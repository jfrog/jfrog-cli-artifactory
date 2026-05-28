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

// MarketplaceEntry is a single plugin record in <harness>-marketplace.json.
// Unknown fields are ignored so the parser stays forward-compatible.
type MarketplaceEntry struct {
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Source  *PluginSource `json:"source,omitempty"`
}

// PluginSource describes where the plugin artifact is stored (only Source.URL is consumed).
type PluginSource struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	SHA    string `json:"sha"`
}

// Marketplace is the on-disk shape of <harness>-marketplace.json.
type Marketplace struct {
	Name    string             `json:"name"`
	Plugins []MarketplaceEntry `json:"plugins"`
}

// MarketplaceFileName returns "<harness>-marketplace.json".
func MarketplaceFileName(harness string) string {
	return strings.ToLower(strings.TrimSpace(harness)) + "-marketplace.json"
}

// DownloadMarketplace downloads <harness>-marketplace.json from the repository root into a temp dir.
// The returned cleanup function removes the temp dir; callers should defer it after consuming the file.
// When the marketplace.json is absent, the error is ErrMarketplaceNotFound (compare with errors.Is).
func DownloadMarketplace(serverDetails *config.ServerDetails, repoKey, harness string) (string, func(), error) {
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

// ParseMarketplace reads and parses a marketplace.json file at path.
func ParseMarketplace(path string) (*Marketplace, error) {
	// #nosec G304 -- path is the marketplace.json we just downloaded into our temp dir.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read marketplace %s: %w", path, err)
	}
	var marketplace Marketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return nil, fmt.Errorf("parse marketplace %s: %w", path, err)
	}
	return &marketplace, nil
}

// FindEntry returns the marketplace entry for the given slug (case-insensitive match on Name).
func FindEntry(marketplace *Marketplace, slug string) (*MarketplaceEntry, bool) {
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
	path, cleanup, err := DownloadMarketplace(serverDetails, repoKey, harness)
	if err != nil {
		return "", err
	}
	defer cleanup()

	marketplace, err := ParseMarketplace(path)
	if err != nil {
		return "", err
	}
	entry, ok := FindEntry(marketplace, slug)
	if !ok {
		return "", fmt.Errorf("plugin '%s' is not listed in %s", slug, MarketplaceFileName(harness))
	}
	version := strings.TrimSpace(entry.Version)
	if version == "" {
		return "", fmt.Errorf("plugin '%s' in %s has no version", slug, MarketplaceFileName(harness))
	}
	return version, nil
}
