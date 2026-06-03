package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

const (
	PackageType = "skills"
	RepoEnvVar  = "JFROG_SKILLS_REPO"
	RepoLabel   = "skills"

	// SearchNamePropertyKey is the Artifactory property key for skills name search (--prop).
	SearchNamePropertyKey = "skill.name"
)

// SearchDescriptionPropertyKeys lists description property keys tried in order after property search.
var SearchDescriptionPropertyKeys = []string{"skill.description"}

func RepoOptions() agentcommon.ResolveRepoOptions {
	return agentcommon.ResolveRepoOptions{
		PackageType: PackageType,
		EnvVar:      RepoEnvVar,
		Label:       RepoLabel,
	}
}
