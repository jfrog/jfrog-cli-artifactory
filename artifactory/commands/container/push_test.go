package container_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container"
	ocicontainer "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPushCommandPointingAt returns a PushCommand wired to talk to testServer.
// The final image tag is "<host:port>/<imagePathWithTag>" so the HEAD request
// issued by GetRepo lands on testServer. imagePathWithTag must already include
// any desired tag (or be left untagged to exercise the implicit ":latest"
// defaulting in GetImageLongNameWithTag).
func newPushCommandPointingAt(t *testing.T, testServer *httptest.Server, imagePathWithTag string) *container.PushCommand {
	t.Helper()
	parsed, err := url.Parse(testServer.URL)
	require.NoError(t, err)

	pc := container.NewPushCommand(ocicontainer.DockerClient)
	pc.SetImageTag(parsed.Host + "/" + imagePathWithTag)
	pc.SetServerDetails(&config.ServerDetails{ArtifactoryUrl: testServer.URL + "/"})
	return pc
}

func TestPushCommand_GetRepo(t *testing.T) {
	// --- Scenarios that mirror the previous TestExtractArtifactoryRepoKey cases ---

	t.Run("returns repo from X-Artifactory-Docker-Registry header for nested image path", func(t *testing.T) {
		// Mirrors original "valid image name": <registry>/<repo>/<image>:<tag>.
		const expectedRepo = "my-repo"
		var hits int32

		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			assert.Equal(t, http.MethodHead, r.Method)
			assert.Equal(t, "/v2/my-repo/my-image/manifests/latest", r.URL.Path)
			w.Header().Set("X-Artifactory-Docker-Registry", expectedRepo)
			w.WriteHeader(http.StatusOK)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "my-repo/my-image:latest")
		gotRepo, err := pc.GetRepo()
		require.NoError(t, err)
		assert.Equal(t, expectedRepo, gotRepo)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "expected exactly one HEAD request to Artifactory")
	})

	t.Run("returns repo for image without explicit tag (defaults to :latest)", func(t *testing.T) {
		// Mirrors original "valid name with no tag": <registry>/<repo>/<image>.
		// GetImageLongNameWithTag should append ":latest" before building the URL.
		const expectedRepo = "my-repo"
		var hits int32

		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			assert.Equal(t, "/v2/my-repo/my-image/manifests/latest", r.URL.Path,
				"a missing tag should default to :latest")
			w.Header().Set("X-Artifactory-Docker-Registry", expectedRepo)
			w.WriteHeader(http.StatusOK)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "my-repo/my-image")
		gotRepo, err := pc.GetRepo()
		require.NoError(t, err)
		assert.Equal(t, expectedRepo, gotRepo)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("returns \"is missing '/'\" error for empty image name", func(t *testing.T) {
		// Mirrors original "error from GetImageLongName (empty)".
		// validateTag rejects the image before any HTTP call is attempted, so no
		// httptest server is required.
		pc := container.NewPushCommand(ocicontainer.DockerClient)
		pc.SetImageTag("")
		pc.SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "http://unused.example/"})

		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "is missing '/'")
	})

	t.Run("returns \"is missing '/'\" error for image name without any slash", func(t *testing.T) {
		// Mirrors original "invalid format with no slash" — pc.GetRepo no longer
		// produces an "invalid image name format" error (that lived in the
		// removed string-parsing helper), but validateTag still rejects any image
		// name that has no '/' separator at all.
		pc := container.NewPushCommand(ocicontainer.DockerClient)
		pc.SetImageTag("registry-only-no-slash")
		pc.SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "http://unused.example/"})

		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "is missing '/'")
	})

	// --- New: reverse-proxy scenario (RTECO-1295) ---

	t.Run("reverse proxy: repo key is in registry hostname, not in image path", func(t *testing.T) {
		// When Artifactory is fronted by a reverse proxy / subdomain layout, the
		// repo key sits in the FRONT of the registry URL (e.g.
		// docker-local.artifactory.example.com/myimage:1.0) instead of being the
		// first path segment after the registry. The image string contains no
		// nested repo segment to parse, so pc.GetRepo() must rely on the
		// X-Artifactory-Docker-Registry response header to discover the repo.
		const expectedRepo = "docker-local"
		var hits int32

		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			assert.Equal(t, http.MethodHead, r.Method)
			// Note the absence of a repo segment in the path — only /v2/<image>/manifests/<tag>.
			assert.Equal(t, "/v2/myimage/manifests/1.0", r.URL.Path)
			w.Header().Set("X-Artifactory-Docker-Registry", expectedRepo)
			w.WriteHeader(http.StatusOK)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "myimage:1.0")
		gotRepo, err := pc.GetRepo()
		require.NoError(t, err)
		assert.Equal(t, expectedRepo, gotRepo,
			"reverse-proxy repo key must come from the response header, not from parsing the image path")
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	// --- pc.GetRepo()-specific behaviours not covered before ---

	t.Run("returns descriptive error on 403 forbidden", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "my-repo/my-image:latest")
		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "403")
		assert.Contains(t, err.Error(), "Possible causes include")
	})

	t.Run("returns error on non-OK status without docker registry header", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "missing-repo/missing-image:latest")
		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "error while getting docker repository name")
	})

	t.Run("returns error when X-Artifactory-Docker-Registry header is missing", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "my-repo/my-image:latest")
		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "X-Artifactory-Docker-Registry")
	})

	t.Run("uses cached repo without making any HTTP call", func(t *testing.T) {
		const cachedRepo = "preset-repo"
		var hits int32

		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer testServer.Close()

		pc := newPushCommandPointingAt(t, testServer, "my-repo/my-image:latest")
		pc.SetRepo(cachedRepo)

		gotRepo, err := pc.GetRepo()
		require.NoError(t, err)
		assert.Equal(t, cachedRepo, gotRepo)
		assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "cached path should not contact Artifactory")
	})

	t.Run("returns error when image is not initialized", func(t *testing.T) {
		pc := container.NewPushCommand(ocicontainer.DockerClient)
		pc.SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "http://unused.example/"})

		gotRepo, err := pc.GetRepo()
		require.Error(t, err)
		assert.Empty(t, gotRepo)
		assert.Contains(t, err.Error(), "container image not initialized")
	})
}
