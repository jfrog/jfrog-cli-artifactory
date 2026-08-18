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

// newRawTokenServer lets a test control the exact response body bytes,
// so we can exercise malformed JSON and missing-field cases.
func newRawTokenServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
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

// TestFetchReferenceToken_MalformedJSON exercises the case where Artifactory
// (or a misconfigured intermediary) responds 200 OK with a body that isn't
// valid JSON at all. The CLI must fail loudly rather than silently writing
// an empty or garbage token into product.json.
func TestFetchReferenceToken_MalformedJSON(t *testing.T) {
	srv := newRawTokenServer(t, http.StatusOK, "<html>not json</html>")
	defer srv.Close()

	details := &config.ServerDetails{Url: srv.URL + "/", ArtifactoryUrl: srv.URL + "/"}
	_, err := FetchReferenceToken(details, "repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse reference token response")
}

// TestFetchReferenceToken_MissingReferenceTokenField mirrors an Artifactory
// response that returns JSON but omits the referenceToken field entirely
// (only repoKey present). Treated as an empty token, so we error out.
func TestFetchReferenceToken_MissingReferenceTokenField(t *testing.T) {
	srv := newRawTokenServer(t, http.StatusOK, `{"repoKey":"repo"}`)
	defer srv.Close()

	details := &config.ServerDetails{Url: srv.URL + "/", ArtifactoryUrl: srv.URL + "/"}
	_, err := FetchReferenceToken(details, "repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty referenceToken")
}
