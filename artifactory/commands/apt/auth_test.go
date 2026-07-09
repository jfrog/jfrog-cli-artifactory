package apt

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── buildSourcesLine ──────────────────────────────────────────────────────────

func TestBuildSourcesLine_Plain(t *testing.T) {
	sd := fakeServerDetails("https://example.jfrog.io/artifactory/", "admin", "secret")
	line, err := buildSourcesLine(sd, "my-repo", "noble", "main", false, "")
	require.NoError(t, err)
	assert.Equal(t, "deb https://admin:secret@example.jfrog.io/artifactory/my-repo noble main", line)
}

func TestBuildSourcesLine_Trusted(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	line, err := buildSourcesLine(sd, "repo", "jammy", "main", true, "")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(line, "deb [trusted=yes] "), "expected [trusted=yes] prefix, got: %s", line)
}

func TestBuildSourcesLine_SignedBy(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	line, err := buildSourcesLine(sd, "repo", "noble", "main", false, "/etc/apt/keyrings/jfrog-repo-noble.asc")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(line, "deb [signed-by=/etc/apt/keyrings/jfrog-repo-noble.asc] "),
		"expected signed-by prefix, got: %s", line)
}

func TestBuildSourcesLine_MultipleComponents(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	line, err := buildSourcesLine(sd, "repo", "noble", "main contrib non-free", false, "")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(line, " noble main contrib non-free"), "got: %s", line)
}

func TestBuildSourcesLine_TrailingSlashStripped(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	line, err := buildSourcesLine(sd, "repo", "noble", "main", false, "")
	require.NoError(t, err)
	assert.NotContains(t, line, "artifactory//repo", "double slash in URL")
}

// ── validateSourcesToken ─────────────────────────────────────────────────────

func TestValidateSourcesToken_Newline(t *testing.T) {
	assert.Error(t, validateSourcesToken("dist", "noble\nevil"))
}

func TestValidateSourcesToken_CR(t *testing.T) {
	assert.Error(t, validateSourcesToken("dist", "noble\revil"))
}

func TestValidateSourcesToken_Null(t *testing.T) {
	assert.Error(t, validateSourcesToken("dist", "noble\x00evil"))
}

func TestValidateSourcesToken_Empty(t *testing.T) {
	assert.Error(t, validateSourcesToken("dist", ""))
}

func TestValidateSourcesToken_Valid(t *testing.T) {
	assert.NoError(t, validateSourcesToken("dist", "noble"))
	assert.NoError(t, validateSourcesToken("component", "main contrib non-free"))
}

// ── WriteTempSourcesList ──────────────────────────────────────────────────────

func TestWriteTempSourcesList_ContainsSourcesLine(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	path, err := WriteTempSourcesList(sd, "repo", "noble", "main", false)
	require.NoError(t, err)
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "deb https://u:p@host/artifactory/repo noble main")
}

func TestWriteTempSourcesList_Permissions(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	path, err := WriteTempSourcesList(sd, "repo", "noble", "main", false)
	require.NoError(t, err)
	defer os.Remove(path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "temp sources.list must not be world-readable")
}

func TestWriteTempSourcesList_Trusted(t *testing.T) {
	sd := fakeServerDetails("https://host/artifactory/", "u", "p")
	path, err := WriteTempSourcesList(sd, "repo", "noble", "main", true)
	require.NoError(t, err)
	defer os.Remove(path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[trusted=yes]")
}

// ── FetchAndInstallPublicKey (HTTP mock) ──────────────────────────────────────

func TestFetchAndInstallPublicKey_AutoDetectsKeyName(t *testing.T) {
	const fakeKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nfakekey\n-----END PGP PUBLIC KEY BLOCK-----\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories/myrepo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"primaryKeyPairRef":"mykey"}`))
		case "/artifactory/api/security/keypair/mykey/public":
			_, _ = w.Write([]byte(fakeKey))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	origKeyringsDir := keyringsDir
	// patch keyringsDir for test isolation
	keyringsDir = tmpDir
	defer func() { keyringsDir = origKeyringsDir }()

	sd := fakeServerDetails(srv.URL+"/artifactory/", "admin", "pass")
	keyPath, err := FetchAndInstallPublicKey(sd, "myrepo", "noble")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "jfrog-myrepo-noble.asc"), keyPath)

	content, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Equal(t, fakeKey, string(content))
}

func TestFetchAndInstallPublicKey_FallsBackToDefaultKey(t *testing.T) {
	const fakeKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\ndefaultkey\n-----END PGP PUBLIC KEY BLOCK-----\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories/myrepo":
			// no primaryKeyPairRef — empty
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rclass":"remote"}`))
		case "/artifactory/api/gpg/key/public":
			_, _ = w.Write([]byte(fakeKey))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	origKeyringsDir := keyringsDir
	keyringsDir = tmpDir
	defer func() { keyringsDir = origKeyringsDir }()

	sd := fakeServerDetails(srv.URL+"/artifactory/", "admin", "pass")
	keyPath, err := FetchAndInstallPublicKey(sd, "myrepo", "noble")
	require.NoError(t, err)
	content, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Equal(t, fakeKey, string(content))
}

func TestFetchAndInstallPublicKey_KeyFilePermissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories/repo":
			_, _ = w.Write([]byte(`{}`))
		case "/artifactory/api/gpg/key/public":
			_, _ = w.Write([]byte("FAKEKEY"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	origKeyringsDir := keyringsDir
	keyringsDir = tmpDir
	defer func() { keyringsDir = origKeyringsDir }()

	sd := fakeServerDetails(srv.URL+"/artifactory/", "u", "p")
	keyPath, err := FetchAndInstallPublicKey(sd, "repo", "noble")
	require.NoError(t, err)

	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "public key must be world-readable (apt needs it)")
}

func TestFetchAndInstallPublicKey_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories/repo":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "forbidden", http.StatusForbidden)
		}
	}))
	defer srv.Close()

	sd := fakeServerDetails(srv.URL+"/artifactory/", "u", "p")
	_, err := FetchAndInstallPublicKey(sd, "repo", "noble")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fakeServerDetails(artURL, user, password string) *config.ServerDetails {
	return &config.ServerDetails{
		ArtifactoryUrl: artURL,
		User:           user,
		Password:       password,
	}
}
