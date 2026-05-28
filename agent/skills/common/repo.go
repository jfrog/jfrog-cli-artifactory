package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

const (
	PackageType = "skills"
	RepoEnvVar  = "JFROG_SKILLS_REPO"
	RepoLabel   = "skills"
)

func RepoOptions() agentcommon.ResolveRepoOptions {
	return agentcommon.ResolveRepoOptions{
		PackageType: PackageType,
		EnvVar:      RepoEnvVar,
		Label:       RepoLabel,
	}
}
