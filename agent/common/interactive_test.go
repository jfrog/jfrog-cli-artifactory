package common

import (
	"os"
	"testing"

	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
)

func TestIsNonInteractive_CITrue(t *testing.T) {
	t.Setenv("CI", "true")
	assert.True(t, IsNonInteractive())
}

func TestIsNonInteractive_CIOne(t *testing.T) {
	t.Setenv("CI", "1")
	assert.True(t, IsNonInteractive())
}

func TestIsNonInteractive_CIFalse(t *testing.T) {
	t.Setenv("CI", "false")
	t.Run("stdout and stdin are terminals", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(true)
		defer revertStdout()

		revertStdin := SetIsStdinTerminal(true)
		defer revertStdin()

		assert.False(t, IsNonInteractive())
	})

	t.Run("stdout is terminal, stdin is not", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(true)
		defer revertStdout()

		revertStdin := SetIsStdinTerminal(false)
		defer revertStdin()

		assert.True(t, IsNonInteractive())
	})

	t.Run("stdout is not terminal", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(false)
		defer revertStdout()
		assert.True(t, IsNonInteractive())
	})
}

func TestIsNonInteractive_CIEmpty(t *testing.T) {
	t.Setenv("CI", "")
	t.Run("stdout and stdin are terminals", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(true)
		defer revertStdout()

		revertStdin := SetIsStdinTerminal(true)
		defer revertStdin()

		assert.False(t, IsNonInteractive())
	})

	t.Run("stdout is terminal, stdin is not", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(true)
		defer revertStdout()

		revertStdin := SetIsStdinTerminal(false)
		defer revertStdin()

		assert.True(t, IsNonInteractive())
	})

	t.Run("stdout is not terminal", func(t *testing.T) {
		revertStdout := log.SetIsTerminalFlagsWithCallback(false)
		defer revertStdout()
		assert.True(t, IsNonInteractive())
	})
}

func TestIsNonInteractive_PipedStdin(t *testing.T) {
	t.Setenv("CI", "")
	revertStdout := log.SetIsTerminalFlagsWithCallback(false)
	defer revertStdout()

	revertStdin := SetIsStdinTerminal(false)
	defer revertStdin()

	assert.True(t, IsNonInteractive(), "piped stdin should be non-interactive")
}

func TestIsNonInteractive_StdinNotTerminal(t *testing.T) {
	t.Setenv("CI", "")
	revertStdout := log.SetIsTerminalFlagsWithCallback(true)
	defer revertStdout()

	revertStdin := SetIsStdinTerminal(false)
	defer revertStdin()

	assert.True(t, IsNonInteractive(), "non-terminal stdin should be non-interactive even if stdout is terminal")
}

func TestIsNonInteractive_CIOverridesTTY(t *testing.T) {
	t.Setenv("CI", "true")
	assert.True(t, IsNonInteractive())
}

func TestPromptLine_Success(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	os.Stdin = r

	go func() {
		defer func() { _ = w.Close() }()
		_, _ = w.WriteString("1.0.0\n")
	}()

	result, err := PromptLine("Enter version: ")
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", result)
}

func TestPromptLine_TrimmsWhitespace(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	os.Stdin = r

	go func() {
		defer func() { _ = w.Close() }()
		_, _ = w.WriteString("  2.5.0  \n")
	}()

	result, err := PromptLine("Version: ")
	assert.NoError(t, err)
	assert.Equal(t, "2.5.0", result)
}

func TestPromptLine_EmptyInput(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	os.Stdin = r

	go func() {
		defer func() { _ = w.Close() }()
		_, _ = w.WriteString("\n")
	}()

	result, err := PromptLine("Enter version: ")
	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestPromptLine_StdinError(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdin = r
	_ = w.Close() // Close writer to simulate EOF/error

	result, err := PromptLine("Enter version: ")
	assert.Error(t, err)
	assert.Equal(t, "", result)
	assert.Contains(t, err.Error(), "read user input")
}
