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
	return IsNonInteractive()
}

// IsNonInteractive returns true when interactive prompts cannot be used safely.
// go-prompt will panic if it tries to read from a non-terminal stdin.
func IsNonInteractive() bool {
	ci := os.Getenv("CI")
	if ci == "true" || ci == "1" {
		return true
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// ShouldFailOnMissingEvidence returns true when quiet/CI mode should fail
// on missing evidence. Default is to fail; set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true
// to override and allow installation without evidence.
func ShouldFailOnMissingEvidence() bool {
	v := os.Getenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE")
	return !strings.EqualFold(v, "true") && v != "1"
}
