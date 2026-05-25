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
