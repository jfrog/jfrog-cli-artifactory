package agentcommon

import (
	"os"
	"strconv"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const envCI = "CI"

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
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func envBool(key string) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && v
}
