package python

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureUvLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := log.GetLogger()
	log.SetLogger(log.NewLogger(log.INFO, buf))
	t.Cleanup(func() { log.SetLogger(prev) })
	return buf
}

// TestUvURLHasEmbeddedCredentials is a table-driven test for uvURLHasEmbeddedCredentials.
// The function returns true when the URL contains a userinfo component (user or user:pass).
func TestUvURLHasEmbeddedCredentials(t *testing.T) {
	cases := []struct {
		url      string
		expected bool
	}{
		{"https://user:pass@host.example.com/path", true},
		{"https://user@host.example.com/path", true}, // user present, no password still counts
		{"https://host.example.com/path", false},
		{"", false},
		{"not-a-url", false},
		{"https://host.example.com/api/pypi/repo/simple", false},
		{"http://user:secret@10.0.0.1:8080/simple", true},
		{"https://:token@host.example.com/simple", true}, // password only
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.expected, uvURLHasEmbeddedCredentials(tc.url),
				"uvURLHasEmbeddedCredentials(%q)", tc.url)
		})
	}
}

// TestUvNetrcHasCredentials writes a temp .netrc file and verifies that
// uvNetrcHasCredentials correctly matches the hostname.
func TestUvNetrcHasCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	err := os.WriteFile(netrcPath, []byte(
		"machine myhost.example.com\nlogin user\npassword pass\n",
	), 0600)
	assert.NoError(t, err)

	// Use NETRC env var so uvNetrcPath() resolves correctly on all platforms
	// (HOME is ignored by os.UserHomeDir on Windows which uses USERPROFILE instead).
	t.Setenv("NETRC", netrcPath)

	// matching host — should return true
	assert.True(t, uvNetrcHasCredentials("https://myhost.example.com/simple"),
		"should match machine entry in .netrc")

	// non-matching host — should return false
	assert.False(t, uvNetrcHasCredentials("https://other.example.com/simple"),
		"should not match when host is absent from .netrc")

	// empty URL — should return false
	assert.False(t, uvNetrcHasCredentials(""),
		"empty URL should return false")

	// URL with no host component — should return false
	assert.False(t, uvNetrcHasCredentials("not-a-url"),
		"unparseable/hostless URL should return false")
}

// TestUvNetrcHasCredentials_CustomPath verifies that the NETRC env var is
// respected for a custom netrc file location (same as UV and curl behavior).
func TestUvNetrcHasCredentials_CustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom_netrc")
	err := os.WriteFile(customPath, []byte("machine customhost.example.com\nlogin u\npassword p\n"), 0600)
	assert.NoError(t, err)

	// Point $NETRC at the custom file, HOME at an empty dir (no ~/.netrc)
	t.Setenv("NETRC", customPath)
	t.Setenv("HOME", t.TempDir())

	assert.True(t, uvNetrcHasCredentials("https://customhost.example.com/simple"),
		"should find credentials via $NETRC custom path")
	assert.False(t, uvNetrcHasCredentials("https://other.example.com/simple"),
		"non-matching host should return false even with $NETRC set")
}

// TestUvNetrcHasCredentials_NoFile verifies that when .netrc does not exist,
// the function returns false rather than erroring.
func TestUvNetrcHasCredentials_NoFile(t *testing.T) {
	// Point NETRC at a non-existent path — works on all platforms.
	t.Setenv("NETRC", filepath.Join(t.TempDir(), ".netrc"))

	assert.False(t, uvNetrcHasCredentials("https://myhost.example.com/simple"),
		"should return false when .netrc file does not exist")
}

// TestUvNetrcHasCredentials_MultipleEntries verifies correct entry selection
// when .netrc contains multiple machine entries.
func TestUvNetrcHasCredentials_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	netrcContent := "machine first.example.com\nlogin u1\npassword p1\n" +
		"machine second.example.com\nlogin u2\npassword p2\n"
	err := os.WriteFile(netrcPath, []byte(netrcContent), 0600)
	assert.NoError(t, err)
	t.Setenv("NETRC", netrcPath)

	assert.True(t, uvNetrcHasCredentials("https://first.example.com/path"))
	assert.True(t, uvNetrcHasCredentials("https://second.example.com/path"))
	assert.False(t, uvNetrcHasCredentials("https://third.example.com/path"))
}

// TestUvIndexEnvName is a table-driven test for uvIndexEnvName.
// UV uppercases the name and replaces hyphens, dots, and spaces with underscores.
func TestUvIndexEnvName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"pypi-virtual", "PYPI_VIRTUAL"},
		{"my.index", "MY_INDEX"},
		{"MyIndex", "MYINDEX"},
		{"index-with-multiple-hyphens", "INDEX_WITH_MULTIPLE_HYPHENS"},
		{"index.with.dots", "INDEX_WITH_DOTS"},
		{"index name", "INDEX_NAME"},
		{"jfrog-pypi-virtual", "JFROG_PYPI_VIRTUAL"},
		{"ALREADY_UPPER", "ALREADY_UPPER"},
		{"mixed.case-name", "MIXED_CASE_NAME"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, uvIndexEnvName(tc.input),
				"uvIndexEnvName(%q)", tc.input)
		})
	}
}

// TestUvIndexHasNativeCredentials verifies the composite logic of
// uvIndexHasNativeCredentials: env var takes priority over URL credentials,
// which take priority over .netrc.
func TestUvIndexHasNativeCredentials(t *testing.T) {
	t.Run("env var set", func(t *testing.T) {
		t.Setenv("UV_INDEX_MY_INDEX_USERNAME", "testuser")
		assert.True(t, uvIndexHasNativeCredentials(
			"https://host.example.com/simple",
			"MY_INDEX",
		))
	})

	t.Run("env var not set, URL has credentials", func(t *testing.T) {
		// ensure env var is not set (t.Setenv restores on cleanup)
		assert.True(t, uvIndexHasNativeCredentials(
			"https://user:pass@host.example.com/simple",
			"NONEXISTENT_VAR_XYZ",
		))
	})

	t.Run("no env var, no URL credentials, netrc match", func(t *testing.T) {
		tmpDir := t.TempDir()
		netrcPath := filepath.Join(tmpDir, ".netrc")
		err := os.WriteFile(netrcPath,
			[]byte("machine netrchost.example.com\nlogin u\npassword p\n"), 0600)
		assert.NoError(t, err)
		t.Setenv("NETRC", netrcPath)
		assert.True(t, uvIndexHasNativeCredentials(
			"https://netrchost.example.com/simple",
			"NONEXISTENT_VAR_XYZ2",
		))
	})

	t.Run("none of the above", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir) // no .netrc
		assert.False(t, uvIndexHasNativeCredentials(
			"https://host.example.com/simple",
			"NONEXISTENT_VAR_XYZ3",
		))
	})
}

func TestUvParseIndexEnvEntry(t *testing.T) {
	cases := []struct{ input, wantName, wantURL string }{
		{"myidx=https://h/simple", "myidx", "https://h/simple"},
		{"  myidx = https://h/simple  ", "myidx", "https://h/simple"},
		{"myidx=https://h/path?q=1&r=2", "myidx", "https://h/path?q=1&r=2"},
		{"my-idx=https://h/simple", "my-idx", "https://h/simple"},
		{"https://h/simple", "", ""},
		{"", "", ""},
		{"=https://h/simple", "", ""},
		{"myidx=", "", ""},
		{"https://h/simple?a=b", "", ""},
		{"a/b=https://h/simple", "", ""},
		{"https://h=x", "", ""},
	}
	for _, tc := range cases {
		entry, ok := uvParseIndexEnvEntry(tc.input)
		assert.Equal(t, tc.wantName != "", ok, tc.input)
		assert.Equal(t, uvIndexEntry{Name: tc.wantName, URL: tc.wantURL}, entry, tc.input)
	}
}

func TestUvReadIndexesFromEnv(t *testing.T) {
	a := uvIndexEntry{Name: "a", URL: "https://a/simple"}
	c := uvIndexEntry{Name: "c", URL: "https://c/simple"}
	def := uvIndexEntry{Name: "def", URL: "https://d/simple", Default: true}
	cases := []struct {
		defaultIndex, index string
		want                []uvIndexEntry
	}{
		{"", "", nil},
		{"def=https://d/simple", "", []uvIndexEntry{def}},
		{"def=https://d/simple", "a=https://a/simple \tc=https://c/simple", []uvIndexEntry{def, a, c}},
		{"https://d/simple", "a=https://a/simple https://b/simple\nc=https://c/simple", []uvIndexEntry{a, c}},
	}
	for _, tc := range cases {
		t.Setenv("UV_DEFAULT_INDEX", tc.defaultIndex)
		t.Setenv("UV_INDEX", tc.index)
		assert.Equal(t, tc.want, uvReadIndexesFromEnv(), "%q / %q", tc.defaultIndex, tc.index)
	}
}

func TestUvReadIndexesFromEnvDoesNotLogCredentials(t *testing.T) {
	t.Setenv("UV_DEFAULT_INDEX", "https://default-token@d.example.com/simple")
	t.Setenv("UV_INDEX", "https://extra-token@i.example.com/simple")
	logOutput := captureUvLog(t)

	uvReadIndexesFromEnv()

	assert.NotContains(t, logOutput.String(), "default-token")
	assert.NotContains(t, logOutput.String(), "extra-token")
	assert.Contains(t, logOutput.String(), "d.example.com")
	assert.Contains(t, logOutput.String(), "i.example.com")
}

func TestUvMergeIndexes(t *testing.T) {
	a := uvIndexEntry{Name: "a", URL: "https://a/simple"}
	b := uvIndexEntry{Name: "b", URL: "https://b/simple"}
	c := uvIndexEntry{Name: "c", URL: "https://c/simple"}
	fromToml := uvIndexEntry{Name: "my-idx", URL: "https://toml/simple"}
	fromEnv := uvIndexEntry{Name: "my-idx", URL: "https://env/simple"}
	sameEnvName := uvIndexEntry{Name: "my.idx", URL: "https://env/simple"}
	cases := []struct{ base, override, want []uvIndexEntry }{
		{[]uvIndexEntry{a}, nil, []uvIndexEntry{a}},
		{nil, []uvIndexEntry{a, b, c}, []uvIndexEntry{a, b, c}},
		{[]uvIndexEntry{a}, []uvIndexEntry{b}, []uvIndexEntry{b, a}},
		{[]uvIndexEntry{fromToml}, []uvIndexEntry{fromEnv}, []uvIndexEntry{fromEnv}},
		{[]uvIndexEntry{fromToml}, []uvIndexEntry{sameEnvName}, []uvIndexEntry{sameEnvName}},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, uvMergeIndexes(tc.base, tc.override))
	}
}

func TestUvResolveBuildInfoIndexURL(t *testing.T) {
	cases := []struct{ defaultIndex, index, legacy, wantVar, wantURL string }{
		{"def=https://d/simple", "a=https://a/simple", "https://l/simple", "UV_DEFAULT_INDEX", "https://d/simple"},
		{"https://d/simple", "", "", "UV_DEFAULT_INDEX", "https://d/simple"},
		{"", "a=https://a/simple b=https://b/simple", "", "UV_INDEX", "https://a/simple"},
		{"", "", "https://l/simple", "UV_INDEX_URL", "https://l/simple"},
		{"", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Setenv("UV_DEFAULT_INDEX", tc.defaultIndex)
		t.Setenv("UV_INDEX", tc.index)
		t.Setenv("UV_INDEX_URL", tc.legacy)
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, tc.wantVar, envVar)
		assert.Equal(t, tc.wantURL, indexURL)
	}
}

func TestInjectCredentials_EnvVarIndexes(t *testing.T) {
	type vars = map[string]string
	const jfIndex = "https://host.example.com/artifactory/api/pypi/repo/simple"
	const otherIndex = "https://other.example.net/simple"
	// pyproject, projectUvToml and userUvToml name an index on otherIndex.
	cases := []struct {
		name                                 string
		env                                  vars
		pyproject, projectUvToml, userUvToml string
		serverID                             string
		want                                 vars
	}{
		{name: "named default index", env: vars{"UV_DEFAULT_INDEX": "myidx=" + jfIndex},
			want: vars{"UV_INDEX_MYIDX_USERNAME": "u", "UV_INDEX_MYIDX_PASSWORD": "p", "UV_KEYRING_PROVIDER": "disabled"}},
		{name: "named UV_INDEX entries", env: vars{"UV_INDEX": "first=" + jfIndex + " second=" + jfIndex},
			want: vars{"UV_INDEX_FIRST_USERNAME": "u", "UV_INDEX_SECOND_PASSWORD": "p"}},
		{name: "named index on other host", env: vars{"UV_DEFAULT_INDEX": "myidx=" + otherIndex},
			want: vars{"UV_INDEX_MYIDX_USERNAME": "", "UV_INDEX_MYIDX_PASSWORD": ""}},
		{name: "named index on other host with server-id", env: vars{"UV_DEFAULT_INDEX": "myidx=" + otherIndex}, serverID: "e2e",
			want: vars{"UV_INDEX_MYIDX_USERNAME": "u", "UV_INDEX_MYIDX_PASSWORD": "p"}},
		{name: "env overrides same-named pyproject index", env: vars{"UV_DEFAULT_INDEX": "myidx=" + jfIndex}, pyproject: "myidx",
			want: vars{"UV_INDEX_MYIDX_USERNAME": "u"}},
		{name: "env vars shared with foreign pyproject index", env: vars{"UV_INDEX": "corp-index=" + jfIndex}, pyproject: "corp_index",
			want: vars{"UV_INDEX_CORP_INDEX_USERNAME": "", "UV_INDEX_CORP_INDEX_PASSWORD": ""}},
		{name: "password-only env credential is kept", env: vars{"UV_DEFAULT_INDEX": "corp=" + jfIndex, "UV_INDEX_CORP_PASSWORD": "shell-token"},
			want: vars{"UV_INDEX_CORP_PASSWORD": "shell-token", "UV_INDEX_CORP_USERNAME": ""}},
		{name: "named index is not renamed", env: vars{"UV_DEFAULT_INDEX": "corp=" + jfIndex},
			want: vars{"UV_DEFAULT_INDEX": "corp=" + jfIndex, "UV_INDEX_CORP_USERNAME": "u", "UV_INDEX_JFROG_USERNAME": ""}},
		{name: "plain default index is named jfrog", env: vars{"UV_DEFAULT_INDEX": jfIndex},
			want: vars{"UV_DEFAULT_INDEX": "jfrog=" + jfIndex, "UV_INDEX_JFROG_USERNAME": "u", "UV_INDEX_JFROG_PASSWORD": "p", "UV_KEYRING_PROVIDER": "disabled"}},
		{name: "plain URL with query string", env: vars{"UV_DEFAULT_INDEX": jfIndex + "?a=b"},
			want: vars{"UV_DEFAULT_INDEX": "jfrog=" + jfIndex + "?a=b", "UV_INDEX_JFROG_USERNAME": "u"}},
		{name: "jfrog credentials already in env", env: vars{"UV_DEFAULT_INDEX": jfIndex, "UV_INDEX_JFROG_USERNAME": "shell-user", "UV_INDEX_JFROG_PASSWORD": "shell-pass"},
			want: vars{"UV_DEFAULT_INDEX": "jfrog-default=" + jfIndex, "UV_INDEX_JFROG_DEFAULT_USERNAME": "u", "UV_INDEX_JFROG_USERNAME": "shell-user"}},
		{name: "pyproject uses jfrog", env: vars{"UV_DEFAULT_INDEX": jfIndex}, pyproject: "jfrog",
			want: vars{"UV_DEFAULT_INDEX": "jfrog-default=" + jfIndex, "UV_INDEX_JFROG_DEFAULT_PASSWORD": "p"}},
		{name: "project uv.toml uses jfrog", env: vars{"UV_DEFAULT_INDEX": jfIndex}, projectUvToml: "jfrog",
			want: vars{"UV_DEFAULT_INDEX": "jfrog-default=" + jfIndex, "UV_INDEX_JFROG_USERNAME": ""}},
		{name: "user uv.toml uses jfrog", env: vars{"UV_DEFAULT_INDEX": jfIndex}, userUvToml: "jfrog",
			want: vars{"UV_DEFAULT_INDEX": "jfrog-default=" + jfIndex, "UV_INDEX_JFROG_USERNAME": ""}},
		{name: "both auto names taken", env: vars{"UV_DEFAULT_INDEX": jfIndex, "UV_INDEX_JFROG_PASSWORD": "x", "UV_INDEX_JFROG_DEFAULT_PASSWORD": "x"},
			want: vars{"UV_DEFAULT_INDEX": jfIndex, "UV_INDEX_JFROG_DEFAULT_USERNAME": ""}},
		{name: "plain URL on other host", env: vars{"UV_DEFAULT_INDEX": otherIndex},
			want: vars{"UV_DEFAULT_INDEX": otherIndex, "UV_INDEX_JFROG_USERNAME": "", "UV_KEYRING_PROVIDER": ""}},
		{name: "plain URL on other host with server-id", env: vars{"UV_DEFAULT_INDEX": otherIndex}, serverID: "e2e",
			want: vars{"UV_DEFAULT_INDEX": "jfrog=" + otherIndex, "UV_INDEX_JFROG_USERNAME": "u"}},
		{name: "plain URL with embedded credentials", env: vars{"UV_DEFAULT_INDEX": "https://user@host.example.com/simple"},
			want: vars{"UV_DEFAULT_INDEX": "https://user@host.example.com/simple", "UV_INDEX_JFROG_USERNAME": ""}},
	}
	writeIndex := func(t *testing.T, path, table, name string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		content := fmt.Sprintf("[[%s]]\nname = %q\nurl = %q\n", table, name, otherIndex)
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"UV_DEFAULT_INDEX", "UV_INDEX", "UV_INDEX_URL", "UV_KEYRING_PROVIDER", "UV_CONFIG_FILE", "XDG_CONFIG_HOME", "APPDATA"} {
				t.Setenv(key, "")
			}
			for _, name := range []string{"MYIDX", "FIRST", "SECOND", "CORP", "CORP_INDEX", "JFROG", "JFROG_DEFAULT"} {
				t.Setenv(uvIndexUsernameKey(name), "")
				t.Setenv(uvIndexPasswordKey(name), "")
			}
			t.Setenv("HOME", t.TempDir())
			t.Setenv("NETRC", filepath.Join(t.TempDir(), "missing-netrc"))
			workDir := t.TempDir()
			if tc.pyproject != "" {
				writeIndex(t, filepath.Join(workDir, "pyproject.toml"), "tool.uv.index", tc.pyproject)
			}
			if tc.projectUvToml != "" {
				writeIndex(t, filepath.Join(workDir, "uv.toml"), "index", tc.projectUvToml)
			}
			if tc.userUvToml != "" {
				configHome := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", configHome)
				writeIndex(t, filepath.Join(configHome, "uv", "uv.toml"), "index", tc.userUvToml)
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			c := &NativeUVCommand{commandName: "install", serverID: tc.serverID}
			c.injectCredentials(workDir, "", &config.ServerDetails{ArtifactoryUrl: "https://host.example.com/artifactory/", User: "u", Password: "p"}, nil)

			for key, value := range tc.want {
				assert.Equal(t, value, os.Getenv(key), key)
			}
		})
	}
}

func TestUvExtractRepoKeyFromURL(t *testing.T) {
	cases := map[string]string{
		"https://host.example.com/artifactory/api/pypi/pypi-remote/simple": "pypi-remote",
		"pypi-local":                         "pypi-local",
		"https://token@host.example.com/":    "",
		"https://host.example.com/simple":    "",
		"https://token@host.example.com/%zz": "",
	}
	for input, want := range cases {
		assert.Equal(t, want, uvExtractRepoKeyFromURL(input), input)
	}
}

func TestUvEnrichDepsForIndex(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	serverDetails := &config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/", AccessToken: "token"}
	deps := []buildinfo.Dependency{{Id: "six:1.17.0"}}

	uvEnrichDepsForIndex(deps, "repo", "https://other.example.net/api/pypi/repo/simple", nil, serverDetails)
	assert.Zero(t, requests.Load(), "index on another host must not be enriched")

	uvEnrichDepsForIndex(deps, "repo", server.URL+"/artifactory/api/pypi/repo/simple", nil, serverDetails)
	assert.NotZero(t, requests.Load(), "index on the server host is enriched")
}
