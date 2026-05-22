package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRepo_FlagTakesPriority(t *testing.T) {
	t.Setenv("JFROG_AGENT_PLUGINS_REPO", "env-repo")
	opts := ResolveRepoOptions{
		PackageType: "agentplugins",
		EnvVar:      "JFROG_AGENT_PLUGINS_REPO",
		Label:       "agent plugins",
	}
	repo, err := ResolveRepo(nil, "flag-repo", true, opts)
	require.NoError(t, err)
	assert.Equal(t, "flag-repo", repo)
}

func TestResolveRepo_EnvFallback(t *testing.T) {
	t.Setenv("JFROG_AGENT_PLUGINS_REPO", "env-repo")
	opts := ResolveRepoOptions{
		PackageType: "agentplugins",
		EnvVar:      "JFROG_AGENT_PLUGINS_REPO",
		Label:       "agent plugins",
	}
	repo, err := ResolveRepo(nil, "", true, opts)
	require.NoError(t, err)
	assert.Equal(t, "env-repo", repo)
}

func TestResolveRepo_EnvNotSet_NoServerDetails(t *testing.T) {
	t.Setenv("JFROG_AGENT_PLUGINS_REPO", "")
	opts := ResolveRepoOptions{
		PackageType: "agentplugins",
		EnvVar:      "JFROG_AGENT_PLUGINS_REPO",
		Label:       "agent plugins",
	}
	_, err := ResolveRepo(nil, "", true, opts)
	assert.Error(t, err)
}
