package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldFailOnMissingEvidenceForSkills_Default(t *testing.T) {
	t.Setenv(EnvDisableQuietFailureSkills, "")
	assert.True(t, ShouldFailOnMissingEvidenceForSkills(), "default should be to fail")
}

func TestShouldFailOnMissingEvidenceForSkills_Disabled(t *testing.T) {
	t.Setenv(EnvDisableQuietFailureSkills, "true")
	assert.False(t, ShouldFailOnMissingEvidenceForSkills())
}

func TestShouldFailOnMissingEvidenceForSkills_OtherValue(t *testing.T) {
	t.Setenv(EnvDisableQuietFailureSkills, "yes")
	assert.True(t, ShouldFailOnMissingEvidenceForSkills(), "'yes' is not a recognized disable value")
}

func TestShouldFailOnMissingEvidenceForPlugins_Default(t *testing.T) {
	t.Setenv(EnvDisableQuietFailurePlugins, "")
	assert.True(t, ShouldFailOnMissingEvidenceForPlugins(), "default should be to fail")
}

func TestShouldFailOnMissingEvidenceForPlugins_Disabled(t *testing.T) {
	t.Setenv(EnvDisableQuietFailurePlugins, "1")
	assert.False(t, ShouldFailOnMissingEvidenceForPlugins())
}

func TestShouldFailOnMissingEvidenceForPlugins_OtherValue(t *testing.T) {
	t.Setenv(EnvDisableQuietFailurePlugins, "yes")
	assert.True(t, ShouldFailOnMissingEvidenceForPlugins(), "'yes' is not a recognized disable value")
}

func TestShouldFailOnMissingEvidence_SkillsEnvDoesNotAffectPlugins(t *testing.T) {
	t.Setenv(EnvDisableQuietFailureSkills, "true")
	t.Setenv(EnvDisableQuietFailurePlugins, "")
	assert.False(t, ShouldFailOnMissingEvidenceForSkills())
	assert.True(t, ShouldFailOnMissingEvidenceForPlugins())
}
