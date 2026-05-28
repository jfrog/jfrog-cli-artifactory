package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectSkillVersion_ExactMatchReturnsIt(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := SelectSkillVersion(available, "1.1.0", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got)
}

func TestSelectSkillVersion_LatestEmpty(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := SelectSkillVersion(available, "", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got)
}

func TestSelectSkillVersion_LatestKeyword(t *testing.T) {
	available := []string{"1.0.0", "3.0.0", "2.0.0"}
	got, err := SelectSkillVersion(available, "latest", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", got)
}

func TestSelectSkillVersion_NotFoundQuiet(t *testing.T) {
	available := []string{"1.0.0", "1.1.0"}
	_, err := SelectSkillVersion(available, "9.9.9", "skills-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectSkillVersion_EmptyAvailableList(t *testing.T) {
	_, err := SelectSkillVersion([]string{}, "", "skills-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest version")
}
