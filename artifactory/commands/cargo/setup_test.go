package cargo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCargoSparseIndexURL(t *testing.T) {
	cases := []struct {
		name, artURL, repo, want string
		wantErr                  bool
	}{
		{name: "trailing slash", artURL: "https://acme.jfrog.io/artifactory/", repo: "cargo-local",
			want: "sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-local/index/"},
		{name: "no trailing slash", artURL: "https://acme.jfrog.io/artifactory", repo: "cargo-remote",
			want: "sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-remote/index/"},
		{name: "empty url", artURL: "", repo: "r", wantErr: true},
		{name: "empty repo", artURL: "https://acme.jfrog.io/artifactory", repo: "", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := cargoSparseIndexURL(c.artURL, c.repo)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestCargoCredential(t *testing.T) {
	// Access token -> Bearer scheme.
	assert.Equal(t, "Bearer abc", cargoCredential(&config.ServerDetails{AccessToken: "abc"}))
	// User + password (no access token) -> Basic scheme with base64(user:password).
	// base64("reshmi:pass") = cmVzaG1pOnBhc3M=
	assert.Equal(t, "Basic cmVzaG1pOnBhc3M=", cargoCredential(&config.ServerDetails{User: "reshmi", Password: "pass"}))
	// Access token wins over basic when both are present.
	assert.Equal(t, "Bearer tok", cargoCredential(&config.ServerDetails{AccessToken: "tok", User: "u", Password: "p"}))
	// Nothing configured -> anonymous.
	assert.Equal(t, "", cargoCredential(&config.ServerDetails{}))
	assert.Equal(t, "", cargoCredential(nil))
	// The credentials.toml value and the env-var value must be identical (both forwarded verbatim).
	assert.Equal(t, []string{"CARGO_REGISTRIES_JFROG_TOKEN=Bearer abc"}, buildAuthEnv("jfrog", "Bearer abc"))
}

// decodeToml reads a TOML file into a generic map for assertions.
func decodeToml(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	m := map[string]interface{}{}
	_, err := toml.DecodeFile(path, &m)
	require.NoError(t, err)
	return m
}

// asTable narrows a TOML value to a nested table, failing the test if the shape is wrong.
// Wrapping the assertion here keeps the tests focused on the config semantics rather than
// on defensive type checks and keeps the linter (forcetypeassert) satisfied.
func asTable(t *testing.T, v interface{}) map[string]interface{} {
	t.Helper()
	m, ok := v.(map[string]interface{})
	require.Truef(t, ok, "expected TOML table, got %T", v)
	return m
}

func TestConfigureNativeRegistry_WritesConfigAndCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CARGO_HOME", home)

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/", AccessToken: "tok123"}
	require.NoError(t, ConfigureNativeRegistry(sd, "cargo-local", ""))

	cfg := decodeToml(t, filepath.Join(home, "config.toml"))
	// [registry] default = "jfrog"
	registry := asTable(t, cfg["registry"])
	assert.Equal(t, jfrogRegistryName, registry["default"])
	// [registries.jfrog] index = sparse+...
	registries := asTable(t, cfg["registries"])
	jfrog := asTable(t, registries[jfrogRegistryName])
	assert.Equal(t, "sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-local/index/", jfrog["index"])
	// [source.crates-io] replace-with = "jfrog"
	source := asTable(t, cfg["source"])
	cratesIo := asTable(t, source["crates-io"])
	assert.Equal(t, jfrogRegistryName, cratesIo["replace-with"])
	// [registry] global-credential-providers = ["cargo:token"] — required for cargo to use the token.
	assert.Equal(t, []interface{}{"cargo:token"}, registry["global-credential-providers"])

	creds := decodeToml(t, filepath.Join(home, "credentials.toml"))
	credRegs := asTable(t, creds["registries"])
	credJfrog := asTable(t, credRegs[jfrogRegistryName])
	// "Bearer " + access token — the scheme Artifactory's Cargo index requires.
	assert.Equal(t, "Bearer tok123", credJfrog["token"])

	// credentials.toml must be 0600 on POSIX. Windows does not use Unix permission bits
	// (os.Stat reports 0666 regardless of the mode passed to os.WriteFile), so the assertion
	// only holds off-Windows.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(home, "credentials.toml"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestConfigureNativeRegistry_WithDeployRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CARGO_HOME", home)

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/", AccessToken: "tok123"}
	// resolve = remote, deploy = local.
	require.NoError(t, ConfigureNativeRegistry(sd, "cargo-remote", "cargo-local"))

	cfg := decodeToml(t, filepath.Join(home, "config.toml"))
	registries := asTable(t, cfg["registries"])
	// Resolution registry (jfrog) -> remote; source replacement targets it.
	jfrog := asTable(t, registries[jfrogRegistryName])
	assert.Equal(t, "sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-remote/index/", jfrog["index"])
	assert.Equal(t, jfrogRegistryName, asTable(t, asTable(t, cfg["source"])["crates-io"])["replace-with"])
	// Deploy registry (jfrog-local) -> local.
	jfrogLocal := asTable(t, registries[jfrogDeployRegistryName])
	assert.Equal(t, "sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-local/index/", jfrogLocal["index"])

	// Both registries get the credential.
	creds := decodeToml(t, filepath.Join(home, "credentials.toml"))
	credRegs := asTable(t, creds["registries"])
	assert.Equal(t, "Bearer tok123", asTable(t, credRegs[jfrogRegistryName])["token"])
	assert.Equal(t, "Bearer tok123", asTable(t, credRegs[jfrogDeployRegistryName])["token"])
}

func TestConfigureNativeRegistry_PreservesExistingKeysAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CARGO_HOME", home)
	require.NoError(t, os.MkdirAll(home, 0755))

	// Pre-existing config with an unrelated registry and a build setting.
	existing := `[build]
jobs = 4

[registries.other]
index = "sparse+https://other.example.com/index/"
`
	configPath := filepath.Join(home, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(existing), 0644))

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory", User: "reshmi", Password: "pw"}
	require.NoError(t, ConfigureNativeRegistry(sd, "repo1", ""))
	// Run twice — must be idempotent.
	require.NoError(t, ConfigureNativeRegistry(sd, "repo1", ""))

	cfg := decodeToml(t, configPath)
	// Unrelated keys preserved.
	build := asTable(t, cfg["build"])
	assert.EqualValues(t, 4, build["jobs"])
	registries := asTable(t, cfg["registries"])
	other := asTable(t, registries["other"])
	assert.Equal(t, "sparse+https://other.example.com/index/", other["index"])
	// Our registry added alongside it.
	jfrog := asTable(t, registries[jfrogRegistryName])
	assert.Equal(t, "sparse+https://acme.jfrog.io/artifactory/api/cargo/repo1/index/", jfrog["index"])
	// Basic auth used when no access token is set: "Basic base64(user:password)".
	// base64("reshmi:pw") = cmVzaG1pOnB3
	creds := decodeToml(t, filepath.Join(home, "credentials.toml"))
	credJfrog := asTable(t, asTable(t, creds["registries"])[jfrogRegistryName])
	assert.Equal(t, "Basic cmVzaG1pOnB3", credJfrog["token"])
}

func TestConfigureNativeRegistry_AnonymousSkipsCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CARGO_HOME", home)

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory"} // no token/password
	require.NoError(t, ConfigureNativeRegistry(sd, "cargo-local", ""))

	// config.toml written, credentials.toml NOT created.
	assert.FileExists(t, filepath.Join(home, "config.toml"))
	_, err := os.Stat(filepath.Join(home, "credentials.toml"))
	assert.True(t, os.IsNotExist(err), "credentials.toml should not exist for anonymous access")
}

func TestConfigureNativeRegistry_NilServerDetails(t *testing.T) {
	assert.Error(t, ConfigureNativeRegistry(nil, "repo", ""))
}
