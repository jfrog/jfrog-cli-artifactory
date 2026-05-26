package common

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeArtifactoryUrl_AppendsArtifactoryPath(t *testing.T) {
	details := &config.ServerDetails{
		ArtifactoryUrl: "https://acme.jfrog.io",
	}
	normalizeArtifactoryUrl(details)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/", details.ArtifactoryUrl)
	assert.Equal(t, "https://acme.jfrog.io/", details.Url)
}

func TestNormalizeArtifactoryUrl_KeepsExistingArtifactoryPath(t *testing.T) {
	details := &config.ServerDetails{
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		Url:            "https://acme.jfrog.io/",
	}
	normalizeArtifactoryUrl(details)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/", details.ArtifactoryUrl)
	assert.Equal(t, "https://acme.jfrog.io/", details.Url)
}

func TestNormalizeArtifactoryUrl_EmptyURL(t *testing.T) {
	details := &config.ServerDetails{}
	normalizeArtifactoryUrl(details)
	assert.Empty(t, details.ArtifactoryUrl)
}

func TestHasServerConfigFlags(t *testing.T) {
	ctx := &components.Context{}
	assert.False(t, hasServerConfigFlags(ctx))

	ctx.AddStringFlag("url", "https://acme.jfrog.io")
	assert.True(t, hasServerConfigFlags(ctx))
}

func TestGetServerDetails_NoConfiguredServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)

	ctx := &components.Context{}
	_, err := GetServerDetails(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default server configured")
}
