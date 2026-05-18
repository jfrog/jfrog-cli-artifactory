package agentcommon

// Constants for the agent plugins package type.
const (
	// AgentPluginsPackageType is the Artifactory package type string used when filtering or
	// describing repositories that host AI agent plugins.
	AgentPluginsPackageType = "agentplugins"

	// AgentPluginsRepoEnvVar names the environment variable consulted to select the
	// agent plugins repository when --repo is not provided.
	AgentPluginsRepoEnvVar = "JFROG_AGENT_PLUGINS_REPO"

	// AgentPluginsRepoLabel is the human-readable label used in prompts and error messages.
	AgentPluginsRepoLabel = "agent plugins"
)

// AgentPluginsRepoOptions returns the canonical ResolveRepoOptions for agent plugins.
func AgentPluginsRepoOptions() ResolveRepoOptions {
	return ResolveRepoOptions{
		PackageType: AgentPluginsPackageType,
		EnvVar:      AgentPluginsRepoEnvVar,
		Label:       AgentPluginsRepoLabel,
	}
}
