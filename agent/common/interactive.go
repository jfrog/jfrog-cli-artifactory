package common

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const envCI = "CI"

// IsQuiet returns true when interactive prompts should be skipped (CI or --quiet).
func IsQuiet(context *components.Context) bool {
	if context.GetBoolFlagValue("quiet") {
		return true
	}
	return IsNonInteractive()
}

// IsNonInteractive returns true when interactive prompts cannot be used safely.
// Checks both CI env var and whether stdin is connected to a terminal.
// go-prompt will panic if it tries to read from a non-terminal stdin.
func IsNonInteractive() bool {
	if IsEnvTrue(envCI) {
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
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
