package common

import (
	"errors"
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// ErrPluginNotFoundInRepo indicates the plugin was not found in the specified repository.
// This is distinct from the repository itself not existing.
var ErrPluginNotFoundInRepo = errors.New("not found in repository")

// listPluginVersions returns the version folders published under <repoKey>/<slug>/ using
// the generic Artifactory storage API. Folder children that are not directories are skipped.
// When a 404 occurs, it disambiguates between missing repo vs missing plugin.
func listPluginVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]string, error) {
	if serverDetails == nil {
		return nil, fmt.Errorf("server details are required to list plugin versions")
	}
	if strings.TrimSpace(repoKey) == "" {
		return nil, fmt.Errorf("repository is required to list plugin versions")
	}
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	info, err := serviceManager.FolderInfo(fmt.Sprintf("%s/%s", repoKey, slug))
	if err != nil {
		// If we got a 404, disambiguate: is it the repo or the plugin that's missing?
		if agentcommon.IsHTTPNotFound(err) {
			// Check if the repo itself exists
			_, repoErr := serviceManager.FolderInfo(repoKey)
			if repoErr != nil && agentcommon.IsHTTPNotFound(repoErr) {
				return nil, fmt.Errorf("repository '%s' not found", repoKey)
			}
			// Repo exists, so it's the plugin that's missing
			// Wrap ErrPluginNotFoundInRepo so errors.Is() can find it for special handling in update.go
			return nil, fmt.Errorf("plugin '%s' not found in repository '%s': %w", slug, repoKey, ErrPluginNotFoundInRepo)
		}
		return nil, err
	}
	versions := make([]string, 0, len(info.Children))
	for _, child := range info.Children {
		if !child.Folder {
			continue
		}
		name := child.Uri
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
		if name == "" {
			continue
		}
		versions = append(versions, name)
	}
	return versions, nil
}

// ResolveLatestPluginVersion returns the greatest semver from listPluginVersions.
func ResolveLatestPluginVersion(serverDetails *config.ServerDetails, repoKey, slug string) (string, error) {
	versions, err := listPluginVersions(serverDetails, repoKey, slug)
	if err != nil {
		return "", fmt.Errorf("failed to list versions for plugin '%s': %w", slug, err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("plugin '%s' has no versions in repository '%s'", slug, repoKey)
	}
	return agentcommon.LatestVersion(versions)
}

// ResolvePluginVersion lists remote versions then applies SelectPackageVersion rules.
// Used by install and update when --version is set or when resolving latest from Artifactory.
// listPluginVersions() already provides specific error messages disambiguating missing repo vs plugin.
func ResolvePluginVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "latest" {
		if err := agentcommon.ValidateSemver(requested); err != nil {
			return "", err
		}
	}
	versions, err := listPluginVersions(serverDetails, repoKey, slug)
	if err != nil {
		// listPluginVersions already disambiguates between missing repo vs missing plugin
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	return agentcommon.SelectPackageVersion(agentcommon.SelectPackageVersionOpts{
		Available: versions,
		Requested: requested,
		RepoKey:   repoKey,
		Quiet:     quiet,
	})
}
