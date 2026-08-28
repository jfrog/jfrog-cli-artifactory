package common

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"golang.org/x/term"
)

// isStdinTerminal checks if stdin is a terminal. Can be mocked for testing.
var isStdinTerminal = defaultIsStdinTerminal

func defaultIsStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) // #nosec G115
}

// SetIsStdinTerminal is a test helper to mock stdin terminal checks.
// It returns a function that reverts the mock when called.
func SetIsStdinTerminal(isTerm bool) func() {
	prev := isStdinTerminal
	isStdinTerminal = func() bool { return isTerm }
	return func() {
		isStdinTerminal = prev
	}
}

const envCI = "CI"

// IsQuiet returns true when interactive prompts should be skipped (CI or --quiet).
func IsQuiet(context *components.Context) bool {
	if context.GetBoolFlagValue("quiet") {
		return true
	}
	return IsNonInteractive()
}

// IsNonInteractive returns true when interactive prompts cannot be used safely.
// Checks CI env var, stdout terminal, and stdin terminal since PromptLine reads/writes both.
func IsNonInteractive() bool {
	if IsEnvTrue(envCI) {
		return true
	}
	if !log.IsStdOutTerminal() {
		return true
	}
	if !isStdinTerminal() {
		return true
	}
	return false
}

// IsEnvTrue reports whether key is set to a truthy value ("true", "1", "t", etc.)
// per strconv.ParseBool. Unset, empty, or invalid values return false.
func IsEnvTrue(key string) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	return err == nil && value
}

// PromptLine prints label to stdout and reads a single trimmed line from stdin.
// Callers should only invoke this when prompts are safe (see IsNonInteractive/IsQuiet).
func PromptLine(label string) (string, error) {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read user input: %w", err)
	}
	return strings.TrimSpace(input), nil
}
