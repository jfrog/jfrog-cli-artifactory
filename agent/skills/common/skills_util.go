package common

import (
	"fmt"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// ResolveSkillVersion lists remote versions then applies SelectPackageVersion rules.
// Uses the fixed ListVersions() which queries raw storage, bypassing the Skills API filter.
// ListVersions() already disambiguates between missing repo vs missing skill on 404 errors.
func ResolveSkillVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		// ListVersions already provides specific error messages for missing repo vs skill
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	available := make([]string, len(versions))
	for idx, skillVersion := range versions {
		available[idx] = skillVersion.Version
	}
	return agentcommon.SelectPackageVersion(agentcommon.SelectPackageVersionOpts{
		Available: available,
		Requested: requested,
		RepoKey:   repoKey,
		Quiet:     quiet,
	})
}
