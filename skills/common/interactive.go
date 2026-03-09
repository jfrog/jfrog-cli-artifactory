package common

import (
	"os"

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
