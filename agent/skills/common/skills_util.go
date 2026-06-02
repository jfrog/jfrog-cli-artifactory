package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// ResolveSkillVersion lists remote versions then applies SelectPackageVersion rules.
func ResolveSkillVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return "", fmt.Errorf("skill '%s' not found in repository '%s'", slug, repoKey)
		}
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	available := make([]string, len(versions))
	for idx, skillVersion := range versions {
		available[idx] = skillVersion.Version
	}
	return agentcommon.SelectPackageVersion(available, strings.TrimSpace(requested), repoKey, quiet)
}

// ResolveLatestSkillVersion returns the greatest semver from ListVersions.
func ResolveLatestSkillVersion(serverDetails *config.ServerDetails, repoKey, slug string) (string, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		return "", fmt.Errorf("failed to list versions for skill '%s': %w", slug, err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("skill '%s' has no versions in repository '%s'", slug, repoKey)
	}
	versionStrs := make([]string, len(versions))
	for idx, skillVersion := range versions {
		versionStrs[idx] = skillVersion.Version
	}
	return agentcommon.LatestVersion(versionStrs)
}
