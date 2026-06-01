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
	return agentcommon.SelectPackageVersion(available, requested, repoKey, quiet)
}

// SelectSkillVersion resolves "" / "latest" / exact match / prompt.
func SelectSkillVersion(available []string, requested, repoKey string, quiet bool) (string, error) {
	return agentcommon.SelectPackageVersion(available, requested, repoKey, quiet)
}
