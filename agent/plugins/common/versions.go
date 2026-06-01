package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// listPluginVersions returns the version folders published under <repoKey>/<slug>/ using
// the generic Artifactory storage API. Folder children that are not directories are skipped.
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
func ResolvePluginVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "latest" {
		if err := ValidateVersion(requested); err != nil {
			return "", err
		}
	}
	versions, err := listPluginVersions(serverDetails, repoKey, slug)
	if err != nil {
		if agentcommon.IsHTTPNotFound(err) {
			return "", fmt.Errorf("plugin '%s' not found in repository '%s'", slug, repoKey)
		}
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	return agentcommon.SelectPackageVersion(agentcommon.SelectPackageVersionOpts{
		Available: versions,
		Requested: requested,
		RepoKey:   repoKey,
		Quiet:     quiet,
	})
}
