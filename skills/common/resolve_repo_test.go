package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRepo_FlagTakesPriority(t *testing.T) {
	t.Setenv("JFROG_SKILLS_REPO", "env-repo")
	repo, err := ResolveRepo(nil, "flag-repo", true)
	require.NoError(t, err)
	assert.Equal(t, "flag-repo", repo)
}

func TestResolveRepo_EnvFallback(t *testing.T) {
	t.Setenv("JFROG_SKILLS_REPO", "env-repo")
	repo, err := ResolveRepo(nil, "", true)
	require.NoError(t, err)
	assert.Equal(t, "env-repo", repo)
}

func TestResolveRepo_EnvNotSet_NoServerDetails(t *testing.T) {
	t.Setenv("JFROG_SKILLS_REPO", "")
	_, err := ResolveRepo(nil, "", true)
	assert.Error(t, err)
}
