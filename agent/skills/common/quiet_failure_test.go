package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
