package common

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPluginVersions_NilServerDetails(t *testing.T) {
	_, err := ListPluginVersions(nil, "repo", "slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details are required")
}

func TestListPluginVersions_EmptyRepoKey(t *testing.T) {
	_, err := ListPluginVersions(&config.ServerDetails{Url: "https://example.com/"}, "", "slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository is required")
}
