package common

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCtx() *components.Context {
	c := &components.Context{}
	c.PrintCommandHelp = func(string) error { return nil }
	return c
}

func TestHasCredentialFlags(t *testing.T) {
	t.Run("empty context is false", func(t *testing.T) {
		assert.False(t, hasCredentialFlags(newCtx()))
	})
	t.Run("--user only is true", func(t *testing.T) {
		c := newCtx()
		c.AddStringFlag("user", "alice")
		assert.True(t, hasCredentialFlags(c))
	})
	t.Run("--password only is true", func(t *testing.T) {
		c := newCtx()
		c.AddStringFlag("password", "secret")
		assert.True(t, hasCredentialFlags(c))
	})
	t.Run("--access-token only is true", func(t *testing.T) {
		c := newCtx()
		c.AddStringFlag("access-token", "TOKEN")
		assert.True(t, hasCredentialFlags(c))
	})
	t.Run("--url alone is NOT a credential flag", func(t *testing.T) {
		c := newCtx()
		c.AddStringFlag("url", "https://acme.jfrog.io/artifactory")
		assert.False(t, hasCredentialFlags(c))
	})
	t.Run("--server-id alone is NOT a credential flag", func(t *testing.T) {
		c := newCtx()
		c.AddStringFlag("server-id", "prod")
		assert.False(t, hasCredentialFlags(c))
	})
}

func TestGetServerDetails_NoConfigNoFlags(t *testing.T) {
	testutil.WithJfrogHome(t)
	_, err := GetServerDetails(newCtx())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default server configured")
}

func TestGetServerDetails_UrlOnlyNoConfig(t *testing.T) {
	testutil.WithJfrogHome(t)
	c := newCtx()
	c.AddStringFlag("url", "https://acme.jfrog.io/artifactory")
	_, err := GetServerDetails(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default server is configured")
	assert.Contains(t, err.Error(), "--url was provided")
}

func TestGetServerDetails_DefaultConfigNoFlags(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		Url:            "https://acme.jfrog.io/",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		User:           "alice",
		Password:       "secret",
		IsDefault:      true,
	})

	got, err := GetServerDetails(newCtx())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/", got.ArtifactoryUrl)
	assert.Equal(t, "alice", got.User)
	assert.Equal(t, "secret", got.Password)
}

// When --url targets the SAME host as the default server, credentials are
// borrowed and the URL from --url is used (allows a base-URL override to
// disambiguate paths like /artifactory vs. /artifactory/ without re-typing
// credentials).
func TestGetServerDetails_UrlSameHost_BorrowsCredsAndOverridesUrl(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		Url:            "https://acme.jfrog.io/",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		User:           "alice",
		Password:       "secret",
		IsDefault:      true,
	})

	c := newCtx()
	overrideUrl := "https://acme.jfrog.io/artifactory"
	c.AddStringFlag("url", overrideUrl)

	got, err := GetServerDetails(c)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, overrideUrl, got.ArtifactoryUrl, "ArtifactoryUrl should be replaced by --url")
	assert.Equal(t, "alice", got.User, "credentials should be borrowed from default config")
	assert.Equal(t, "secret", got.Password, "credentials should be borrowed from default config")
}

// When --url targets a DIFFERENT host than the default server, we must NOT
// reuse the saved credentials — return an error asking for explicit creds.
func TestGetServerDetails_UrlDifferentHost_ErrorsOut(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		Url:            "https://acme.jfrog.io/",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		User:           "alice",
		Password:       "secret",
		IsDefault:      true,
	})

	c := newCtx()
	c.AddStringFlag("url", "https://other.jfrog.io/artifactory")

	_, err := GetServerDetails(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different host")
	assert.Contains(t, err.Error(), "other.jfrog.io")
	assert.Contains(t, err.Error(), "acme.jfrog.io")
}

func TestGetServerDetails_ExplicitCredsRouteToFlagPath(t *testing.T) {
	testutil.WithJfrogHome(t)
	saveDefaultServer(t, &config.ServerDetails{
		ServerId:       "default",
		Url:            "https://acme.jfrog.io/",
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		User:           "should-not-be-used",
		Password:       "should-not-be-used",
		IsDefault:      true,
	})

	c := newCtx()
	c.AddStringFlag("url", "https://other.jfrog.io/artifactory")
	c.AddStringFlag("access-token", "flag-token")

	got, err := GetServerDetails(c)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "flag-token", got.AccessToken, "flag token should be used verbatim")
	assert.Empty(t, got.User, "default-config user must not leak into explicit-creds path")
	assert.Empty(t, got.Password, "default-config password must not leak into explicit-creds path")
}

func saveDefaultServer(t *testing.T, srv *config.ServerDetails) {
	t.Helper()
	require.NoError(t, config.SaveServersConf([]*config.ServerDetails{srv}))
}

func TestNormalizeArtifactoryBaseUrl(t *testing.T) {
	tests := []struct {
		name      string
		rawUrl    string
		repoKey   string
		wantUrl   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty url passes through",
			rawUrl:  "",
			repoKey: "vscode-remote",
			wantUrl: "",
		},
		{
			name:    "unparseable url passes through",
			rawUrl:  "://not a url",
			repoKey: "vscode-remote",
			wantUrl: "://not a url",
		},
		{
			name:    "no host passes through",
			rawUrl:  "/artifactory/foo",
			repoKey: "foo",
			wantUrl: "/artifactory/foo",
		},
		{
			name:    "clean base url with artifactory suffix, no repo key",
			rawUrl:  "https://acme.jfrog.io/artifactory",
			repoKey: "",
			wantUrl: "https://acme.jfrog.io/artifactory",
		},
		{
			name:    "clean base url with artifactory suffix and repo key",
			rawUrl:  "https://acme.jfrog.io/artifactory",
			repoKey: "vscode-remote",
			wantUrl: "https://acme.jfrog.io/artifactory",
		},
		{
			name:    "clean base url with trailing slash preserved",
			rawUrl:  "https://acme.jfrog.io/artifactory/",
			repoKey: "vscode-remote",
			wantUrl: "https://acme.jfrog.io/artifactory/",
		},
		{
			name:    "host only, no path",
			rawUrl:  "https://acme.jfrog.io",
			repoKey: "vscode-remote",
			wantUrl: "https://acme.jfrog.io",
		},
		{
			name:    "custom prefix ending in artifactory",
			rawUrl:  "https://internal.corp/tools/artifactory",
			repoKey: "vscode-remote",
			wantUrl: "https://internal.corp/tools/artifactory",
		},
		{
			name:    "reported bug: trailing segment matches repo key is stripped",
			rawUrl:  "https://tompazus.jfrog.io/artifactory/vscode-remote",
			repoKey: "vscode-remote",
			wantUrl: "https://tompazus.jfrog.io/artifactory",
		},
		{
			name:    "trailing segment matches repo key with trailing slash",
			rawUrl:  "https://acme.jfrog.io/artifactory/my-repo/",
			repoKey: "my-repo",
			wantUrl: "https://acme.jfrog.io/artifactory/",
		},
		{
			name:    "case-insensitive match on trailing segment",
			rawUrl:  "https://acme.jfrog.io/artifactory/MY-REPO",
			repoKey: "my-repo",
			wantUrl: "https://acme.jfrog.io/artifactory",
		},
		{
			name:    "trailing repo strip preserves port and query",
			rawUrl:  "https://acme.jfrog.io:8443/artifactory/vscode-remote?tag=x",
			repoKey: "vscode-remote",
			wantUrl: "https://acme.jfrog.io:8443/artifactory?tag=x",
		},
		{
			name:      "api url is rejected",
			rawUrl:    "https://acme.jfrog.io/artifactory/api/repositories/foo",
			repoKey:   "foo",
			wantErr:   true,
			errSubstr: "full API/service URL",
		},
		{
			name:      "aieditorextensions gallery url is rejected",
			rawUrl:    "https://acme.jfrog.io/artifactory/api/aieditorextensions/vscode-remote/_apis/public/gallery",
			repoKey:   "vscode-remote",
			wantErr:   true,
			errSubstr: "full API/service URL",
		},
		{
			name:      "extra segment after artifactory not matching repo key",
			rawUrl:    "https://acme.jfrog.io/artifactory/some-other-repo",
			repoKey:   "vscode-remote",
			wantErr:   true,
			errSubstr: "path segment after '/artifactory/'",
		},
		{
			name:      "extra segment after artifactory with no repo key",
			rawUrl:    "https://acme.jfrog.io/artifactory/some-repo",
			repoKey:   "",
			wantErr:   true,
			errSubstr: "path segment after '/artifactory/'",
		},
		{
			name:      "deep path after artifactory not matching repo key",
			rawUrl:    "https://acme.jfrog.io/artifactory/some-repo/nested/path",
			repoKey:   "vscode-remote",
			wantErr:   true,
			errSubstr: "path segment after '/artifactory/'",
		},
		{
			name:    "no artifactory anchor, no api, unusual path is left alone",
			rawUrl:  "https://internal.corp/tools/foo",
			repoKey: "vscode-remote",
			wantUrl: "https://internal.corp/tools/foo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeArtifactoryBaseUrl(tc.rawUrl, tc.repoKey)
			if tc.wantErr {
				assert.Error(t, err, "expected an error for %q", tc.rawUrl)
				if err != nil && tc.errSubstr != "" {
					assert.Contains(t, err.Error(), tc.errSubstr, "error text mismatch")
				}
				return
			}
			assert.NoError(t, err, "unexpected error for %q", tc.rawUrl)
			assert.Equal(t, tc.wantUrl, got)
		})
	}
}

func TestGetBaseUrl(t *testing.T) {
	tests := []struct {
		name    string
		details *config.ServerDetails
		want    string
	}{
		{
			name:    "artifactory url wins over url",
			details: &config.ServerDetails{ArtifactoryUrl: "https://a.jfrog.io/artifactory/", Url: "https://a.jfrog.io/"},
			want:    "https://a.jfrog.io/artifactory",
		},
		{
			name:    "falls back to url when artifactory url empty",
			details: &config.ServerDetails{Url: "https://a.jfrog.io/"},
			want:    "https://a.jfrog.io",
		},
		{
			name:    "both empty returns empty",
			details: &config.ServerDetails{},
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, GetBaseUrl(tc.details))
		})
	}
}

func TestExtractRepoKeyFromURL(t *testing.T) {
	tests := []struct {
		name    string
		urlStr  string
		apiType string
		want    string
	}{
		{
			name:    "extracts repo from repositories api",
			urlStr:  "https://acme.jfrog.io/artifactory/api/repositories/my-repo",
			apiType: "repositories",
			want:    "my-repo",
		},
		{
			name:    "extracts repo from aieditorextensions gallery",
			urlStr:  "https://acme.jfrog.io/artifactory/api/aieditorextensions/vscode-remote/_apis/public/gallery",
			apiType: "aieditorextensions",
			want:    "vscode-remote",
		},
		{
			name:    "api type not present",
			urlStr:  "https://acme.jfrog.io/artifactory/api/other/my-repo",
			apiType: "aieditorextensions",
			want:    "",
		},
		{
			name:    "api type at end returns empty",
			urlStr:  "https://acme.jfrog.io/artifactory/api/aieditorextensions",
			apiType: "aieditorextensions",
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExtractRepoKeyFromURL(tc.urlStr, tc.apiType))
		})
	}
}

func TestIsValidUrl(t *testing.T) {
	assert.True(t, IsValidUrl("https://acme.jfrog.io/artifactory"))
	assert.True(t, IsValidUrl("http://localhost:8081/artifactory"))
	assert.False(t, IsValidUrl(""))
	assert.False(t, IsValidUrl("not-a-url"))
	assert.False(t, IsValidUrl("/only/path"))
}

func TestBuildURL(t *testing.T) {
	base := "https://acme.jfrog.io/artifactory"
	assert.Equal(t,
		base+"/api/aieditorextensions/vscode-remote",
		BuildURL(base, "aieditorextensions", "vscode-remote", ""))
	assert.Equal(t,
		base+"/api/aieditorextensions/vscode-remote/_apis/public/gallery",
		BuildURL(base, "aieditorextensions", "vscode-remote", "_apis/public/gallery"))
	assert.Equal(t,
		base+"/api/aieditorextensions/vscode-remote/_apis/public/gallery",
		BuildURL(base, "aieditorextensions", "vscode-remote", "/_apis/public/gallery"))
}
