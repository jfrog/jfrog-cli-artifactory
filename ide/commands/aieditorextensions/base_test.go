package aieditorextensions

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCtx(args ...string) *components.Context {
	c := &components.Context{Arguments: args}
	c.PrintCommandHelp = func(string) error { return nil }
	return c
}

func saveDefaultServer(t *testing.T, srv *config.ServerDetails) {
	t.Helper()
	require.NoError(t, config.SaveServersConf([]*config.ServerDetails{srv}))
}

func TestParseBaseSetupConfig_DirectPositionalURL(t *testing.T) {
	testutil.WithJfrogHome(t)

	// arg[0] is IDE_NAME; arg[1] is SERVICE_URL (per the CLI convention).
	ctx := newCtx(
		"vscode",
		"https://acme.jfrog.io/artifactory/api/aieditorextensions/my-repo/_apis/public/gallery",
	)

	cfg, err := ParseBaseSetupConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.IsDirectURL, "positional URL should yield IsDirectURL=true")
	assert.Equal(t, "my-repo", cfg.RepoKey)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/aieditorextensions/my-repo/_apis/public/gallery", cfg.ServiceURL)
	assert.Nil(t, cfg.ServerDetails, "positional URL path never consults server config")
}

func TestParseBaseSetupConfig_MissingRepoKey(t *testing.T) {
	testutil.WithJfrogHome(t)
	ctx := newCtx("vscode")
	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--repo-key flag is required")
}

func TestParseBaseSetupConfig_UrlAlone_NoConfig(t *testing.T) {
	testutil.WithJfrogHome(t)
	ctx := newCtx("vscode")
	ctx.AddStringFlag("url", "https://acme.jfrog.io/artifactory")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default server is configured")
	assert.NotContains(t, err.Error(), "does not exist", "should not surface a misleading not-found error")
}

func TestParseBaseSetupConfig_BuggyUrl_ApiUrl(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		AccessToken:    "fake-token",
		IsDefault:      true,
	})

	ctx := newCtx("vscode")
	ctx.AddStringFlag("url", "https://acme.jfrog.io/artifactory/api/aieditorextensions/my-repo/_apis/public/gallery")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "full API/service URL")
}

func TestParseBaseSetupConfig_BuggyUrl_ExtraSegment(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		AccessToken:    "fake-token",
		IsDefault:      true,
	})

	ctx := newCtx("vscode")
	ctx.AddStringFlag("url", "https://acme.jfrog.io/artifactory/some-other-repo")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path segment after '/artifactory/'")
	assert.Contains(t, err.Error(), "some-other-repo")
	assert.Contains(t, err.Error(), "my-repo", "error should mention the mismatched --repo-key")
}

// Same-host --url + default config: credentials are borrowed and validation
// is attempted (fails because the fake token isn't a real one, but the flow
// must reach the validation stage rather than tripping any of our early URL
// or config errors).
func TestParseBaseSetupConfig_UrlAlone_DefaultConfig_ReachesValidation(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		AccessToken:    "fake-token",
		IsDefault:      true,
	})

	ctx := newCtx("vscode")
	// --url points at the SAME host as the default server → creds are borrowed.
	ctx.AddStringFlag("url", "https://acme.jfrog.io/artifactory")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "full API/service URL")
	assert.NotContains(t, err.Error(), "path segment after '/artifactory/'")
	assert.NotContains(t, err.Error(), "no default server")
	assert.NotContains(t, err.Error(), "different host")
}

// Off-host --url + default config: refuse to reuse saved credentials on a
// host the user didn't associate them with.
func TestParseBaseSetupConfig_UrlDifferentHost_Errors(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		AccessToken:    "fake-token",
		IsDefault:      true,
	})

	ctx := newCtx("vscode")
	ctx.AddStringFlag("url", "https://other-tenant.example/artifactory")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different host")
	assert.Contains(t, err.Error(), "other-tenant.example")
}

// Reporter's buggy URL shape (base + repo appended) targeting the same host
// as the default server. The normalizer must strip the trailing repo BEFORE
// any network call, and the same-host check must pass so credentials get
// borrowed and validation is attempted.
func TestParseBaseSetupConfig_BuggyUrl_NormalizedBeforeValidation(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		AccessToken:    "fake-token",
		IsDefault:      true,
	})

	ctx := newCtx("vscode")
	ctx.AddStringFlag("url", "https://acme.jfrog.io/artifactory/my-repo")
	ctx.AddStringFlag(RepoKeyFlag, "my-repo")

	_, err := ParseBaseSetupConfig(ctx)
	require.Error(t, err)
	// Should reach validation, not any of the early error paths.
	assert.NotContains(t, err.Error(), "path segment after '/artifactory/'")
	assert.NotContains(t, err.Error(), "full API/service URL")
	assert.NotContains(t, err.Error(), "different host")
	assert.NotContains(t, err.Error(), "no default server")
}

func TestParseBaseSetupConfig_Constants(t *testing.T) {
	assert.Equal(t, "aieditorextensions", ApiType)
	assert.Equal(t, "_apis/public/gallery", DefaultURLSuffix)
	assert.Equal(t, "repo-key", RepoKeyFlag)
	assert.Equal(t, "url-suffix", URLSuffixFlag)
	assert.Equal(t, "product-json-path", ProductJsonPath)
}
