package common

import (
	"os"
	"testing"

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
	// When CI is not truthy, result depends on whether stdin is a terminal.
	_ = IsNonInteractive()
}

func TestIsNonInteractive_CIEmpty(t *testing.T) {
	t.Setenv("CI", "")
	_ = IsNonInteractive()
}

func TestIsNonInteractive_PipedStdin(t *testing.T) {
	t.Setenv("CI", "")

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() {
		os.Stdin = origStdin
		_ = r.Close() // test teardown
		_ = w.Close() // test teardown
	}()

	os.Stdin = r
	assert.True(t, IsNonInteractive(), "piped stdin should be non-interactive")
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
