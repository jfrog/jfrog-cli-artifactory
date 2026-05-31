package common

const (
	// EnvDisableQuietFailureSkills allows skills install without evidence in quiet/CI mode.
	EnvDisableQuietFailureSkills = "JFROG_SKILLS_DISABLE_QUIET_FAILURE"
	// EnvDisableQuietFailurePlugins allows plugins install without evidence in quiet/CI mode.
	EnvDisableQuietFailurePlugins = "JFROG_PLUGINS_DISABLE_QUIET_FAILURE"
)

// ShouldFailOnMissingEvidenceForSkills returns true when quiet/CI mode should fail on missing evidence.
// Default is to fail. Set EnvDisableQuietFailureSkills to true to allow installation without evidence.
func ShouldFailOnMissingEvidenceForSkills() bool {
	return !IsEnvTrue(EnvDisableQuietFailureSkills)
}

// ShouldFailOnMissingEvidenceForPlugins returns true when quiet/CI mode should fail on missing evidence.
// Default is to fail. Set EnvDisableQuietFailurePlugins to true to allow installation without evidence.
func ShouldFailOnMissingEvidenceForPlugins() bool {
	return !IsEnvTrue(EnvDisableQuietFailurePlugins)
}

// DisableQuietFailureEvidenceHintForSkills describes how to override fail-fast evidence checks for skills.
func DisableQuietFailureEvidenceHintForSkills() string {
	return "Set " + EnvDisableQuietFailureSkills + "=true to proceed without evidence"
}

// DisableQuietFailureEvidenceHintForPlugins describes how to override fail-fast evidence checks for plugins.
func DisableQuietFailureEvidenceHintForPlugins() string {
	return "Set " + EnvDisableQuietFailurePlugins + "=true to proceed without evidence"
}
