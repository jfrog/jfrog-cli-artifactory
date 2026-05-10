package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde relative path", in: "~/x/y", want: filepath.Join(home, "x/y")},
		{name: "claude skills path", in: "~/.claude/skills", want: filepath.Join(home, ".claude/skills")},
		{name: "absolute path", in: "/abs/path", want: "/abs/path"},
		{name: "absolute long path", in: "/absolute/path/skills", want: "/absolute/path/skills"},
		{name: "tilde only unchanged", in: "~", want: "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandHome(tt.in))
		})
	}
}

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
