package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

const envDisableQuietFailure = "JFROG_SKILLS_DISABLE_QUIET_FAILURE"

// ShouldFailOnMissingEvidence returns true when quiet/CI mode should fail
// on missing evidence. Default is to fail; set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true
// to override and allow installation without evidence.
func ShouldFailOnMissingEvidence() bool {
	return !agentcommon.IsEnvTrue(envDisableQuietFailure)
}
