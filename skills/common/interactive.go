package common

import (
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// IsQuiet returns true when interactive prompts should be skipped (CI or --quiet).
func IsQuiet(c *components.Context) bool {
	if c.GetBoolFlagValue("quiet") {
		return true
	}
	return IsCI()
}

// IsCI returns true when running in a CI environment.
func IsCI() bool {
	ci := os.Getenv("CI")
	return ci == "true" || ci == "1"
}

// ShouldFailOnMissingEvidence returns true when quiet/CI mode should fail
// on missing evidence. Default is to fail; set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true
// to override and allow installation without evidence.
func ShouldFailOnMissingEvidence() bool {
	v := os.Getenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE")
	return !strings.EqualFold(v, "true") && v != "1"
}
