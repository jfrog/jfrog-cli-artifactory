package alpine

import (
	"net/http"
	"net/http/httptest"
	"testing"

	biUtils "github.com/jfrog/build-info-go/build/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── stripJFFlags ─────────────────────────────────────────────────────────────

func TestStripJFFlags_RemovesBuildNameAndNumber(t *testing.T) {
	args := []string{"add", "--build-name", "my-build", "--build-number", "1", "curl"}
	got := stripJFFlags(args)
	assert.Equal(t, []string{"add", "curl"}, got)
}

func TestStripJFFlags_RemovesEqualFormFlags(t *testing.T) {
	args := []string{"add", "--build-name=my-build", "--repo=alpine-local", "curl"}
	got := stripJFFlags(args)
	assert.Equal(t, []string{"add", "curl"}, got)
}

func TestStripJFFlags_PreservesNativeFlags(t *testing.T) {
	args := []string{"add", "--no-cache", "--update-cache", "curl"}
	got := stripJFFlags(args)
	assert.Equal(t, []string{"add", "--no-cache", "--update-cache", "curl"}, got)
}

func TestStripJFFlags_AllJFFlags(t *testing.T) {
	args := []string{
		"add",
		"--build-name", "b", "--build-number", "1",
		"--project", "proj", "--module", "mod",
		"--server-id", "sid", "--repo", "r",
		"--alpine-version", "v3.20",
		"--user", "admin", "--password", "pass",
		"curl",
	}
	got := stripJFFlags(args)
	assert.Equal(t, []string{"add", "curl"}, got)
}

func TestStripJFFlags_EmptyArgs(t *testing.T) {
	assert.Empty(t, stripJFFlags(nil))
	assert.Empty(t, stripJFFlags([]string{}))
}

func TestAlpineModuleID(t *testing.T) {
	assert.Equal(t, "alpine-virtual:x86_64:v3.21",
		alpineModuleID("", "alpine-virtual", "x86_64", "3.21"))
	assert.Equal(t, "alpine-local:aarch64:v3.20",
		alpineModuleID("", "alpine-local", "aarch64", "v3.20"))
	assert.Equal(t, "custom-module",
		alpineModuleID("custom-module", "alpine-local", "x86_64", "v3.21"))
	assert.Equal(t, "apk:unknown:unknown",
		alpineModuleID("", "", "", ""))
}

func TestApkFileNameFromID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string
	}{
		{name: "name and version", id: "curl:8.5.0-r0", expected: "curl-8.5.0-r0.apk"},
		{name: "version with revision", id: "musl:1.2.4-r2", expected: "musl-1.2.4-r2.apk"},
		{name: "only first colon replaced", id: "so:libc:1.0", expected: "so-libc:1.0.apk"},
		{name: "no colon", id: "curl", expected: "curl.apk"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, apkFileNameFromID(tc.id))
		})
	}
}

// ─── extractPackageNames ──────────────────────────────────────────────────────

func TestExtractPackageNames_BasicPackages(t *testing.T) {
	args := []string{"curl", "git", "bash"}
	assert.Equal(t, []string{"curl", "git", "bash"}, extractPackageNames(args))
}

func TestExtractPackageNames_SkipsFlags(t *testing.T) {
	args := []string{"--no-cache", "curl", "--update-cache", "git"}
	assert.Equal(t, []string{"curl", "git"}, extractPackageNames(args))
}

func TestExtractPackageNames_Empty(t *testing.T) {
	assert.Nil(t, extractPackageNames(nil))
	assert.Nil(t, extractPackageNames([]string{"--no-cache"}))
}

// ─── matchGlob ────────────────────────────────────────────────────────────────

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern  string
		input    string
		expected bool
	}{
		{"*password*", "my_password_123", true},
		// matchGlob is case-sensitive; filterSecretEnvVars lowercases the name before calling it
		{"*password*", "MY_PASSWORD", false},
		{"*secret*", "aws_secret_key", true},
		{"*token*", "access_token", true},
		{"*key*", "api_key", true},
		{"*password*", "username", false},
		{"exact", "exact", true},
		{"exact", "notexact", false},
		{"*", "anything", true},
		{"prefix*", "prefix_suffix", true},
		{"prefix*", "no_prefix", false},
		{"*suffix", "some_suffix", true},
		{"*suffix", "suffix_more", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+"/"+tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, matchGlob(tc.pattern, tc.input))
		})
	}
}

// ─── filterSecretEnvVars ──────────────────────────────────────────────────────

func TestFilterSecretEnvVars_RemovesSecrets(t *testing.T) {
	env := []string{
		"HOME=/root",
		"MY_PASSWORD=secret123",
		"ACCESS_TOKEN=tok",
		"API_KEY=abc",
		"SOME_SECRET=xyz",
		"PATH=/usr/bin",
	}
	filtered := filterSecretEnvVars(env)
	assert.Contains(t, filtered, "HOME=/root")
	assert.Contains(t, filtered, "PATH=/usr/bin")
	assert.NotContains(t, filtered, "MY_PASSWORD=secret123")
	assert.NotContains(t, filtered, "ACCESS_TOKEN=tok")
	assert.NotContains(t, filtered, "API_KEY=abc")
	assert.NotContains(t, filtered, "SOME_SECRET=xyz")
}

func TestFilterSecretEnvVars_PreservesNonSecrets(t *testing.T) {
	env := []string{"HOME=/root", "USER=admin", "GOPATH=/go"}
	filtered := filterSecretEnvVars(env)
	assert.Equal(t, env, filtered)
}

// ─── buildHTTPAuth ────────────────────────────────────────────────────────────

func TestBuildHTTPAuth_Standard(t *testing.T) {
	auth, err := buildHTTPAuth("https://myrt.jfrog.io/artifactory", "admin", "pass123")
	require.NoError(t, err)
	assert.Equal(t, "basic:myrt.jfrog.io:admin:pass123", auth)
}

func TestBuildHTTPAuth_TrailingSlash(t *testing.T) {
	auth, err := buildHTTPAuth("https://rt.example.com/", "user", "pwd")
	require.NoError(t, err)
	assert.Equal(t, "basic:rt.example.com:user:pwd", auth)
}

func TestBuildHTTPAuth_InvalidURL(t *testing.T) {
	_, err := buildHTTPAuth("://bad-url", "user", "pwd")
	assert.Error(t, err)
}

// ─── excludeRequestedPackages ─────────────────────────────────────────────────

func TestExcludeRequestedPackages_NilSnapshot(t *testing.T) {
	result := excludeRequestedPackages(nil, []string{"curl"})
	assert.Nil(t, result, "nil snapshot should return nil")
}

func TestExcludeRequestedPackages_NoRequestedNames(t *testing.T) {
	result := excludeRequestedPackages(nil, nil)
	assert.Nil(t, result)
}

func TestExcludeRequestedPackages_RemovesRequested(t *testing.T) {
	snapshot := []biUtils.AlpinePackage{
		{Name: "curl", Version: "8.5.0-r0"},
		{Name: "musl", Version: "1.2.4-r2"},
	}
	result := excludeRequestedPackages(snapshot, []string{"curl"})
	require.Len(t, result, 1)
	assert.Equal(t, "musl", result[0].Name)
}

func TestExcludeRequestedPackages_IgnoresFlagLikeTokens(t *testing.T) {
	snapshot := []biUtils.AlpinePackage{{Name: "my-build", Version: "1.0-r0"}}
	result := excludeRequestedPackages(snapshot, []string{"curl"})
	require.Len(t, result, 1)
	assert.Equal(t, "my-build", result[0].Name)
}

func TestAqlJSONString_EscapesQuotes(t *testing.T) {
	assert.Equal(t, `"repo\"key"`, aqlJSONString(`repo"key`))
	assert.Equal(t, `"curl-8.5.0-r0.apk"`, aqlJSONString("curl-8.5.0-r0.apk"))
}

func TestEmitSignatureHint_DetectsPatterns(t *testing.T) {
	emitSignatureHint("UNTRUSTED signature for APKINDEX")
	emitSignatureHint("everything is fine")
}

func TestFilterSecretEnvVars_CommaSeparatedPatterns(t *testing.T) {
	t.Setenv("JFROG_CLI_ENV_EXCLUDE", "*password*,*token*")
	env := []string{"HOME=/root", "DB_PASSWORD=x", "ACCESS_TOKEN=y", "PATH=/bin"}
	filtered := filterSecretEnvVars(env)
	assert.Contains(t, filtered, "HOME=/root")
	assert.Contains(t, filtered, "PATH=/bin")
	assert.NotContains(t, filtered, "DB_PASSWORD=x")
	assert.NotContains(t, filtered, "ACCESS_TOKEN=y")
}

func TestFilterSecretEnvVars_DefaultIncludesPswAndAuth(t *testing.T) {
	t.Setenv("JFROG_CLI_ENV_EXCLUDE", "")
	env := []string{"HOME=/root", "DB_PSW=x", "MY_AUTH=y"}
	filtered := filterSecretEnvVars(env)
	assert.Contains(t, filtered, "HOME=/root")
	assert.NotContains(t, filtered, "DB_PSW=x")
	assert.NotContains(t, filtered, "MY_AUTH=y")
}

// ─── resolveCredentials ───────────────────────────────────────────────────────

func TestResolveCredentials_OverrideWins(t *testing.T) {
	sd := &config.ServerDetails{}
	sd.SetUser("stored-user")
	sd.SetPassword("stored-pass")

	user, pass := resolveCredentials(sd, "override-user", "override-pass")
	assert.Equal(t, "override-user", user)
	assert.Equal(t, "override-pass", pass)
}

func TestResolveCredentials_FallsBackToStored(t *testing.T) {
	sd := &config.ServerDetails{}
	sd.SetUser("stored-user")
	sd.SetPassword("stored-pass")

	user, pass := resolveCredentials(sd, "", "")
	assert.Equal(t, "stored-user", user)
	assert.Equal(t, "stored-pass", pass)
}

func TestResolveCredentials_NilServerDetails(t *testing.T) {
	user, pass := resolveCredentials(nil, "u", "p")
	assert.Equal(t, "u", user)
	assert.Equal(t, "p", pass)
}

// ─── ensureRepoExists ────────────────────────────────────────────────────────

func TestEnsureRepoExists_ExistingRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/repositories/alpine-local", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverDetails := &config.ServerDetails{ArtifactoryUrl: server.URL + "/"}
	require.NoError(t, ensureRepoExists("alpine-local", serverDetails))
}

func TestEnsureRepoExists_MissingRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	serverDetails := &config.ServerDetails{ArtifactoryUrl: server.URL + "/"}
	err := ensureRepoExists("missing-repo", serverDetails)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository 'missing-repo' not found")
}

func TestEnsureRepoExists_RequiresConfiguredServer(t *testing.T) {
	err := ensureRepoExists("alpine-local", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no JFrog server configured")
}

// ─── resolveHTTPAuthCredentials ───────────────────────────────────────────────

func TestResolveHTTPAuthCredentials_OverrideWins(t *testing.T) {
	sd := &config.ServerDetails{}
	sd.SetUser("stored")
	sd.SetPassword("stored-pass")

	user, pass := resolveHTTPAuthCredentials(sd, "flag-user", "flag-pass")
	assert.Equal(t, "flag-user", user)
	assert.Equal(t, "flag-pass", pass)
}

func TestResolveHTTPAuthCredentials_FallsBackToStored(t *testing.T) {
	sd := &config.ServerDetails{}
	sd.SetUser("stored-user")
	sd.SetPassword("stored-pass")

	user, pass := resolveHTTPAuthCredentials(sd, "", "")
	assert.Equal(t, "stored-user", user)
	assert.Equal(t, "stored-pass", pass)
}

func TestResolveHTTPAuthCredentials_NilServerDetails(t *testing.T) {
	user, pass := resolveHTTPAuthCredentials(nil, "u", "p")
	assert.Equal(t, "u", user)
	assert.Equal(t, "p", pass)
}

func TestBuildInfoSubcmds(t *testing.T) {
	assert.True(t, buildInfoSubcmds["add"])
	assert.True(t, buildInfoSubcmds["upgrade"])
	assert.False(t, buildInfoSubcmds["upload"], "upload is handled by ApkUploadCommand, not ApkCommand")
	assert.False(t, buildInfoSubcmds["update"])
	assert.False(t, buildInfoSubcmds["fetch"])
	assert.False(t, buildInfoSubcmds["search"])
	assert.False(t, buildInfoSubcmds["del"])
	assert.False(t, buildInfoSubcmds["info"])
}

func TestUnsupportedBuildInfoMessage(t *testing.T) {
	msg := unsupportedBuildInfoMessage("update")
	assert.Contains(t, msg, "apk update")
	assert.Contains(t, msg, "not available")
	assert.Contains(t, msg, "add")
	assert.Contains(t, msg, "upgrade")
	assert.Contains(t, msg, "upload")
	assert.Contains(t, msg, "passthrough")
}
