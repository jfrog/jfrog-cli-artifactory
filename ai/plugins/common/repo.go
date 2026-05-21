package common

import (
	aicommon "github.com/jfrog/jfrog-cli-artifactory/ai/common"
)

// Constants for the agent plugins package type.
const (
	// PackageType is the Artifactory package type string used when filtering or
	// describing repositories that host AI agent plugins.
	PackageType = "agentplugins"

	// RepoEnvVar names the environment variable consulted to select the agent plugins
	// repository when --repo is not provided.
	RepoEnvVar = "JFROG_AGENT_PLUGINS_REPO"

	// RepoLabel is the human-readable label used in prompts and error messages.
	RepoLabel = "agent plugins"
)

// RepoOptions returns the canonical ResolveRepoOptions for agent plugins.
func RepoOptions() aicommon.ResolveRepoOptions {
	return aicommon.ResolveRepoOptions{
		PackageType: PackageType,
		EnvVar:      RepoEnvVar,
		Label:       RepoLabel,
	}
}
