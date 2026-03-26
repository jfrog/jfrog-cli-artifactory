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
	// In test runners, stdin is typically a pipe (non-interactive).
	// We just verify it doesn't panic.
	_ = IsNonInteractive()
}

func TestIsNonInteractive_CIEmpty(t *testing.T) {
	t.Setenv("CI", "")
	// Falls through to stdin check. In test runners stdin is usually a pipe.
	_ = IsNonInteractive()
}

func TestIsNonInteractive_PipedStdin(t *testing.T) {
	t.Setenv("CI", "")

	// Save original stdin and replace with a pipe (non-terminal).
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() {
		os.Stdin = origStdin
		_ = r.Close()
		_ = w.Close()
	}()

	os.Stdin = r
	assert.True(t, IsNonInteractive(), "piped stdin should be non-interactive")
}

func TestIsNonInteractive_CIOverridesTTY(t *testing.T) {
	// Even if stdin were a terminal, CI=true should return true immediately.
	t.Setenv("CI", "true")
	assert.True(t, IsNonInteractive())
}

func TestShouldFailOnMissingEvidence_Default(t *testing.T) {
	t.Setenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE", "")
	assert.True(t, ShouldFailOnMissingEvidence(), "default should be to fail")
}

func TestShouldFailOnMissingEvidence_DisabledTrue(t *testing.T) {
	t.Setenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE", "true")
	assert.False(t, ShouldFailOnMissingEvidence())
}

func TestShouldFailOnMissingEvidence_DisabledTrueUppercase(t *testing.T) {
	t.Setenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE", "TRUE")
	assert.False(t, ShouldFailOnMissingEvidence())
}

func TestShouldFailOnMissingEvidence_DisabledOne(t *testing.T) {
	t.Setenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE", "1")
	assert.False(t, ShouldFailOnMissingEvidence())
}

func TestShouldFailOnMissingEvidence_OtherValue(t *testing.T) {
	t.Setenv("JFROG_SKILLS_DISABLE_QUIET_FAILURE", "yes")
	assert.True(t, ShouldFailOnMissingEvidence(), "'yes' is not a recognized disable value")
}
