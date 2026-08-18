package aieditorextensions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTokenServer(t *testing.T, wantRepoKey string, respondWith generateTokenResponse, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/"+aiEditorExtensionTokenAPI) {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("repoKey"); got != wantRepoKey {
			t.Errorf("repoKey mismatch: got %q want %q", got, wantRepoKey)
		}
		w.WriteHeader(status)
		if respondWith.ReferenceToken != "" || respondWith.RepoKey != "" {
			_ = json.NewEncoder(w).Encode(respondWith)
		}
	}))
}

func TestFetchReferenceToken_Success(t *testing.T) {
	srv := newTokenServer(t, "adam-aiee-remote",
		generateTokenResponse{ReferenceToken: "abc123", RepoKey: "adam-aiee-remote"},
		http.StatusOK)
	defer srv.Close()

	details := &config.ServerDetails{Url: srv.URL + "/", ArtifactoryUrl: srv.URL + "/"}
	token, err := FetchReferenceToken(details, "adam-aiee-remote")
	require.NoError(t, err)
	assert.Equal(t, "abc123", token)
}

func TestFetchReferenceToken_EmptyRepoKey(t *testing.T) {
	details := &config.ServerDetails{Url: "http://example.invalid/", ArtifactoryUrl: "http://example.invalid/"}
	_, err := FetchReferenceToken(details, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository key is required")
}

func TestFetchReferenceToken_NilServerDetails(t *testing.T) {
	_, err := FetchReferenceToken(nil, "some-repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details are required")
}

func TestFetchReferenceToken_ServerError(t *testing.T) {
	srv := newTokenServer(t, "repo", generateTokenResponse{}, http.StatusInternalServerError)
	defer srv.Close()

	details := &config.ServerDetails{Url: srv.URL + "/", ArtifactoryUrl: srv.URL + "/"}
	_, err := FetchReferenceToken(details, "repo")
	require.Error(t, err)
}

func TestFetchReferenceToken_EmptyToken(t *testing.T) {
	srv := newTokenServer(t, "repo",
		generateTokenResponse{ReferenceToken: "", RepoKey: "repo"},
		http.StatusOK)
	defer srv.Close()

	details := &config.ServerDetails{Url: srv.URL + "/", ArtifactoryUrl: srv.URL + "/"}
	_, err := FetchReferenceToken(details, "repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty referenceToken")
}

func TestAppendReferenceToken(t *testing.T) {
	tests := []struct {
		name, url, token, want string
	}{
		{"no trailing slash", "https://acme/api/aieditorextensions/repo/_apis/public/gallery", "T", "https://acme/api/aieditorextensions/repo/_apis/public/gallery/T"},
		{"trailing slash", "https://acme/api/aieditorextensions/repo/_apis/public/gallery/", "T", "https://acme/api/aieditorextensions/repo/_apis/public/gallery/T"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, AppendReferenceToken(tc.url, tc.token))
		})
	}
}

