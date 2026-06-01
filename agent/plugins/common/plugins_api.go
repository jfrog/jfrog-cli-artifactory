package common

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// PluginListItem is a single entry returned by ListPlugins.
type PluginListItem struct {
	Slug          string
	LatestVersion string
}

// ListPlugins lists all plugin slugs in repoKey by reading the root folder's children,
// resolves the latest version for each, sorts by name, and applies limit.
func ListPlugins(serverDetails *config.ServerDetails, repoKey string, limit int) ([]PluginListItem, error) {
	if serverDetails == nil {
		return nil, fmt.Errorf("server details are required to list plugins")
	}
	if strings.TrimSpace(repoKey) == "" {
		return nil, fmt.Errorf("repository is required to list plugins")
	}

	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}

	info, err := serviceManager.FolderInfo(repoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to list plugins in repository '%s': %w", repoKey, err)
	}

	pluginEntries := make([]PluginListItem, 0, len(info.Children))
	for _, child := range info.Children {
		if !child.Folder {
			continue
		}
		slug := strings.TrimPrefix(child.Uri, "/")
		if slug == "" || strings.HasPrefix(slug, ".") {
			continue
		}
		latest, err := ResolveLatestPluginVersion(serverDetails, repoKey, slug)
		if err != nil {
			log.Warn(fmt.Sprintf("Could not resolve latest version for plugin '%s': %s", slug, err.Error()))
			latest = ""
		}
		pluginEntries = append(pluginEntries, PluginListItem{Slug: slug, LatestVersion: latest})
	}

	sort.Slice(pluginEntries, func(i, j int) bool {
		return strings.ToLower(pluginEntries[i].Slug) < strings.ToLower(pluginEntries[j].Slug)
	})

	if limit > 0 && len(pluginEntries) > limit {
		pluginEntries = pluginEntries[:limit]
	}

	return pluginEntries, nil
}

// ReadPluginMeta reads the primary plugin.json under pluginDir and returns the parsed PluginMeta.
func ReadPluginMeta(pluginDir string) (PluginMeta, error) {
	_, meta, err := findPrimaryPluginManifest(pluginDir)
	return meta, err
}
