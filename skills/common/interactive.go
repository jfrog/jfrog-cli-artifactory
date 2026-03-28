package common

import (
	"os"
	"strconv"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	// envCI is the standard CI environment variable set by most CI systems.
	envCI = "CI"
	// envDisableQuietFailure controls whether quiet/CI mode fails on missing evidence.
	envDisableQuietFailure = "JFROG_SKILLS_DISABLE_QUIET_FAILURE"
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
	if envBool(envCI) {
		return true
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	// If ModeCharDevice is NOT set, stdin is piped or redirected (non-interactive).
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// ShouldFailOnMissingEvidence returns true when quiet/CI mode should fail
// on missing evidence. Default is to fail; set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true
// to override and allow installation without evidence.
func ShouldFailOnMissingEvidence() bool {
	return !envBool(envDisableQuietFailure)
}

// envBool returns true if the named environment variable is set to a truthy value
// ("true", "TRUE", "1", "t", "T", "yes", "YES", etc.) as defined by strconv.ParseBool.
func envBool(key string) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && v
}
