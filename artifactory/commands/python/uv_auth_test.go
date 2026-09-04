package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			"UV_INDEX_MY_INDEX_USERNAME",
		))
	})

	t.Run("env var not set, URL has credentials", func(t *testing.T) {
		// ensure env var is not set (t.Setenv restores on cleanup)
		assert.True(t, uvIndexHasNativeCredentials(
			"https://user:pass@host.example.com/simple",
			"UV_INDEX_NONEXISTENT_VAR_XYZ",
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
			"UV_INDEX_NONEXISTENT_VAR_XYZ2",
		))
	})

	t.Run("none of the above", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir) // no .netrc
		assert.False(t, uvIndexHasNativeCredentials(
			"https://host.example.com/simple",
			"UV_INDEX_NONEXISTENT_VAR_XYZ3",
		))
	})
}

// TestUvParseIndexEnvEntry covers the "[name=]url" parser used for UV_INDEX
// and UV_DEFAULT_INDEX entries.
func TestUvParseIndexEnvEntry(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantOK   bool
		wantName string
		wantURL  string
	}{
		{"named", "myidx=https://h/simple", true, "myidx", "https://h/simple"},
		{"trims surrounding whitespace", "  myidx = https://h/simple  ", true, "myidx", "https://h/simple"},
		{"no equals sign", "https://h/simple", false, "", ""},
		{"empty input", "", false, "", ""},
		{"empty name", "=https://h/simple", false, "", ""},
		{"empty url", "myidx=", false, "", ""},
		{"url contains equals", "myidx=https://h/path?q=1&r=2", true, "myidx", "https://h/path?q=1&r=2"},
		{"hyphen in name preserved", "my-idx=https://h/simple", true, "my-idx", "https://h/simple"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := uvParseIndexEnvEntry(tc.input)
			assert.Equal(t, tc.wantOK, ok, "ok mismatch for %q", tc.input)
			assert.Equal(t, tc.wantName, entry.Name, "name mismatch for %q", tc.input)
			assert.Equal(t, tc.wantURL, entry.URL, "url mismatch for %q", tc.input)
		})
	}
}

// TestUvReadIndexesFromEnv covers parsing of UV_DEFAULT_INDEX and UV_INDEX
// into uvIndexEntry slices, including unnamed/empty handling.
func TestUvReadIndexesFromEnv(t *testing.T) {
	// Make sure no inherited env leaks across sub-tests.
	clearUvIndexEnv := func(t *testing.T) {
		t.Setenv("UV_DEFAULT_INDEX", "")
		t.Setenv("UV_INDEX", "")
	}

	t.Run("default only, named", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "def=https://d.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 1)
		assert.Equal(t, "def", got[0].Name)
		assert.Equal(t, "https://d.example.com/simple", got[0].URL)
		assert.True(t, got[0].Default, "UV_DEFAULT_INDEX entries are flagged Default")
	})

	t.Run("index only, single", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 1)
		assert.Equal(t, "a", got[0].Name)
		assert.False(t, got[0].Default)
	})

	t.Run("index only, multiple spaces", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple    b=https://b.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "b", got[1].Name)
	})

	t.Run("tabs and mixed whitespace", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple\tb=https://b.example.com/simple\nc=https://c.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 3)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "b", got[1].Name)
		assert.Equal(t, "c", got[2].Name)
	})

	t.Run("default and index combined", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "def=https://d.example.com/simple")
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple b=https://b.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 3)
		assert.Equal(t, "def", got[0].Name, "default entry comes first")
		assert.True(t, got[0].Default)
		assert.Equal(t, "a", got[1].Name)
		assert.Equal(t, "b", got[2].Name)
	})

	t.Run("unnamed default skipped", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "https://d.example.com/simple")
		got := uvReadIndexesFromEnv()
		assert.Empty(t, got, "unnamed UV_DEFAULT_INDEX cannot be authenticated via per-index env vars")
	})

	t.Run("mixed named and unnamed in UV_INDEX", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple https://b.example.com/simple c=https://c.example.com/simple")
		got := uvReadIndexesFromEnv()
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "c", got[1].Name)
	})

	t.Run("both empty", func(t *testing.T) {
		clearUvIndexEnv(t)
		got := uvReadIndexesFromEnv()
		assert.Empty(t, got)
	})
}

// TestUvMergeIndexes covers the precedence rules: env entries first (in their
// declared order), then non-conflicting base entries; dedup by canonical env name.
func TestUvMergeIndexes(t *testing.T) {
	t.Run("env overrides toml same name", func(t *testing.T) {
		base := []uvIndexEntry{{Name: "myidx", URL: "https://toml.example.com/simple"}}
		override := []uvIndexEntry{{Name: "myidx", URL: "https://env.example.com/simple"}}
		got := uvMergeIndexes(base, override)
		require.Len(t, got, 1)
		assert.Equal(t, "https://env.example.com/simple", got[0].URL)
	})

	t.Run("toml-only kept", func(t *testing.T) {
		base := []uvIndexEntry{{Name: "a", URL: "https://a.example.com/simple"}}
		got := uvMergeIndexes(base, nil)
		assert.Equal(t, base, got)
	})

	t.Run("env-only kept", func(t *testing.T) {
		override := []uvIndexEntry{{Name: "b", URL: "https://b.example.com/simple"}}
		got := uvMergeIndexes(nil, override)
		require.Len(t, got, 1)
		assert.Equal(t, "b", got[0].Name)
	})

	t.Run("env first then toml", func(t *testing.T) {
		base := []uvIndexEntry{{Name: "a", URL: "https://a.example.com/simple"}}
		override := []uvIndexEntry{{Name: "b", URL: "https://b.example.com/simple"}}
		got := uvMergeIndexes(base, override)
		require.Len(t, got, 2)
		assert.Equal(t, "b", got[0].Name)
		assert.Equal(t, "a", got[1].Name)
	})

	t.Run("dedup by canonical env name", func(t *testing.T) {
		// "my-idx" and "my.idx" both canonicalize to "MY_IDX" via uvIndexEnvName.
		base := []uvIndexEntry{{Name: "my-idx", URL: "https://toml.example.com/simple"}}
		override := []uvIndexEntry{{Name: "my.idx", URL: "https://env.example.com/simple"}}
		got := uvMergeIndexes(base, override)
		require.Len(t, got, 1, "entries collapsing to the same UV env-var name should dedupe")
		assert.Equal(t, "https://env.example.com/simple", got[0].URL)
	})

	t.Run("preserves env declaration order", func(t *testing.T) {
		override := []uvIndexEntry{
			{Name: "a", URL: "https://a.example.com/simple"},
			{Name: "b", URL: "https://b.example.com/simple"},
			{Name: "c", URL: "https://c.example.com/simple"},
		}
		got := uvMergeIndexes(nil, override)
		require.Len(t, got, 3)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "b", got[1].Name)
		assert.Equal(t, "c", got[2].Name)
	})
}

// TestUvResolveBuildInfoIndexURL covers the env-var fallback chain used by
// build-info: UV_DEFAULT_INDEX → first UV_INDEX entry → UV_INDEX_URL.
func TestUvResolveBuildInfoIndexURL(t *testing.T) {
	clearEnv := func(t *testing.T) {
		t.Setenv("UV_DEFAULT_INDEX", "")
		t.Setenv("UV_INDEX", "")
		t.Setenv("UV_INDEX_URL", "")
	}

	t.Run("UV_DEFAULT_INDEX named wins", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "def=https://d.example.com/simple")
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple")
		t.Setenv("UV_INDEX_URL", "https://legacy.example.com/simple")
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, "UV_DEFAULT_INDEX", envVar)
		assert.Equal(t, "https://d.example.com/simple", indexURL, "name= prefix should be stripped")
	})

	t.Run("UV_DEFAULT_INDEX unnamed falls back to raw URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "https://d.example.com/simple")
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, "UV_DEFAULT_INDEX", envVar)
		assert.Equal(t, "https://d.example.com/simple", indexURL)
	})

	t.Run("UV_INDEX used when no UV_DEFAULT_INDEX", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("UV_INDEX", "a=https://a.example.com/simple b=https://b.example.com/simple")
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, "UV_INDEX", envVar)
		assert.Equal(t, "https://a.example.com/simple", indexURL, "first named UV_INDEX entry wins")
	})

	t.Run("UV_INDEX_URL legacy fallback", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("UV_INDEX_URL", "https://legacy.example.com/simple")
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, "UV_INDEX_URL", envVar)
		assert.Equal(t, "https://legacy.example.com/simple", indexURL)
	})

	t.Run("all unset", func(t *testing.T) {
		clearEnv(t)
		envVar, indexURL := uvResolveBuildInfoIndexURL()
		assert.Equal(t, "", envVar)
		assert.Equal(t, "", indexURL)
	})
}

// TestInjectCredentials_EnvVarIndexes exercises NativeUVCommand.injectCredentials
// end-to-end with UV_INDEX / UV_DEFAULT_INDEX set, verifying that credentials are
// injected into UV_INDEX_<NAME>_USERNAME/PASSWORD env vars.
func TestInjectCredentials_EnvVarIndexes(t *testing.T) {
	newServer := func() *config.ServerDetails {
		return &config.ServerDetails{
			ArtifactoryUrl: "https://host.example.com/artifactory/",
			User:           "u",
			Password:       "p",
		}
	}

	// Each sub-test clears the cred env vars it will check, plus the source
	// vars, so prior state cannot leak in or out.
	clearUvIndexEnv := func(t *testing.T, names ...string) {
		t.Setenv("UV_DEFAULT_INDEX", "")
		t.Setenv("UV_INDEX", "")
		t.Setenv("UV_INDEX_URL", "")
		t.Setenv("UV_KEYRING_PROVIDER", "")
		for _, n := range names {
			t.Setenv("UV_INDEX_"+n+"_USERNAME", "")
			t.Setenv("UV_INDEX_"+n+"_PASSWORD", "")
		}
		// Point HOME / NETRC at empty dirs so uvNetrcHasCredentials returns false.
		t.Setenv("HOME", t.TempDir())
		t.Setenv("NETRC", filepath.Join(t.TempDir(), "missing-netrc"))
	}

	t.Run("UV_DEFAULT_INDEX named, host matches", func(t *testing.T) {
		clearUvIndexEnv(t, "MYIDX")
		t.Setenv("UV_DEFAULT_INDEX", "myidx=https://host.example.com/artifactory/api/pypi/repo/simple")

		c := &NativeUVCommand{commandName: "install"}
		c.injectCredentials(t.TempDir(), "", newServer(), nil)

		assert.Equal(t, "u", os.Getenv("UV_INDEX_MYIDX_USERNAME"))
		assert.Equal(t, "p", os.Getenv("UV_INDEX_MYIDX_PASSWORD"))
		assert.Equal(t, "disabled", os.Getenv("UV_KEYRING_PROVIDER"))
	})

	t.Run("UV_INDEX with two named entries", func(t *testing.T) {
		clearUvIndexEnv(t, "FIRST", "SECOND")
		t.Setenv("UV_INDEX", "first=https://host.example.com/artifactory/api/pypi/a/simple second=https://host.example.com/artifactory/api/pypi/b/simple")

		c := &NativeUVCommand{commandName: "install"}
		c.injectCredentials(t.TempDir(), "", newServer(), nil)

		assert.Equal(t, "u", os.Getenv("UV_INDEX_FIRST_USERNAME"))
		assert.Equal(t, "p", os.Getenv("UV_INDEX_FIRST_PASSWORD"))
		assert.Equal(t, "u", os.Getenv("UV_INDEX_SECOND_USERNAME"))
		assert.Equal(t, "p", os.Getenv("UV_INDEX_SECOND_PASSWORD"))
	})

	t.Run("UV_DEFAULT_INDEX unnamed is skipped", func(t *testing.T) {
		clearUvIndexEnv(t)
		t.Setenv("UV_DEFAULT_INDEX", "https://host.example.com/artifactory/api/pypi/repo/simple")

		c := &NativeUVCommand{commandName: "install"}
		c.injectCredentials(t.TempDir(), "", newServer(), nil)

		// No name → no UV_INDEX_<NAME>_USERNAME could be derived → keyring provider
		// must not have been set as a side effect either.
		assert.Equal(t, "", os.Getenv("UV_KEYRING_PROVIDER"))
	})

	t.Run("host mismatch without server-id skips injection", func(t *testing.T) {
		clearUvIndexEnv(t, "MYIDX")
		t.Setenv("UV_DEFAULT_INDEX", "myidx=https://other.example.net/api/pypi/repo/simple")

		c := &NativeUVCommand{commandName: "install"} // no serverID → no host override
		c.injectCredentials(t.TempDir(), "", newServer(), nil)

		assert.Equal(t, "", os.Getenv("UV_INDEX_MYIDX_USERNAME"), "host mismatch must skip injection")
		assert.Equal(t, "", os.Getenv("UV_INDEX_MYIDX_PASSWORD"))
	})

	t.Run("host mismatch with explicit server-id injects anyway", func(t *testing.T) {
		clearUvIndexEnv(t, "MYIDX")
		t.Setenv("UV_DEFAULT_INDEX", "myidx=https://other.example.net/api/pypi/repo/simple")

		c := &NativeUVCommand{commandName: "install", serverID: "explicit"}
		c.injectCredentials(t.TempDir(), "", newServer(), nil)

		assert.Equal(t, "u", os.Getenv("UV_INDEX_MYIDX_USERNAME"), "explicit --server-id overrides host mismatch")
		assert.Equal(t, "p", os.Getenv("UV_INDEX_MYIDX_PASSWORD"))
	})

	t.Run("env override wins over conflicting pyproject toml", func(t *testing.T) {
		clearUvIndexEnv(t, "MYIDX")
		// pyproject.toml has myidx pointing at a host that doesn't match the server.
		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "pyproject.toml"), []byte(`
[[tool.uv.index]]
name = "myidx"
url = "https://stale.example.net/simple"
`), 0600))
		// UV_DEFAULT_INDEX overrides with a matching host.
		t.Setenv("UV_DEFAULT_INDEX", "myidx=https://host.example.com/artifactory/api/pypi/repo/simple")

		c := &NativeUVCommand{commandName: "install"}
		c.injectCredentials(workDir, "", newServer(), nil)

		assert.Equal(t, "u", os.Getenv("UV_INDEX_MYIDX_USERNAME"),
			"env-var URL host (matches) should win over pyproject.toml URL host (mismatch)")
	})
}
