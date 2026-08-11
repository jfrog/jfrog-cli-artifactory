package cargo

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// jfrogRegistryName is the cargo registry name written by `jf setup cargo` for dependency
// resolution. A fixed name keeps setup idempotent and is the single target of the
// [source.crates-io] redirect (which can only point at one source).
const jfrogRegistryName = "jfrog"

// jfrogDeployRegistryName is the cargo registry name written for publishing. Cargo can only redirect
// crates.io resolution to one source (jfrogRegistryName, a remote), so the publish target — a local
// repo — is written as a second named registry. Users publish with `cargo publish --registry jfrog-local`.
const jfrogDeployRegistryName = "jfrog-local"

// cargoHome returns the cargo home directory: $CARGO_HOME if set, else ~/.cargo.
func cargoHome() (string, error) {
	if h := os.Getenv("CARGO_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errorutils.CheckErrorf("failed to resolve home directory for cargo config: %s", err.Error())
	}
	return filepath.Join(home, ".cargo"), nil
}

// cargoSparseIndexURL builds the Artifactory sparse index URL for a cargo repo:
//
//	sparse+https://<host>/artifactory/api/cargo/<repo>/index/
//
// artifactoryURL is the server's Artifactory URL (already includes the /artifactory path).
func cargoSparseIndexURL(artifactoryURL, repoName string) (string, error) {
	if artifactoryURL == "" {
		return "", errorutils.CheckErrorf("artifactory URL is empty; cannot build cargo index URL")
	}
	if repoName == "" {
		return "", errorutils.CheckErrorf("repository name is empty; cannot build cargo index URL")
	}
	base := strings.TrimSuffix(artifactoryURL, "/")
	full := fmt.Sprintf("%s/api/cargo/%s/index/", base, repoName)
	if _, err := url.Parse(full); err != nil {
		return "", errorutils.CheckErrorf("failed to build cargo index URL: %s", err.Error())
	}
	return "sparse+" + full, nil
}

// ConfigureNativeRegistry writes the JFrog Artifactory cargo registry and credentials into the
// user-level cargo config, so that plain `cargo` (not just `jf cargo`) resolves and authenticates
// against Artifactory. This is the persistent counterpart to the per-run env-var injection
// (resolveAuthEnv); the two are complementary.
//
// ~/.cargo/config.toml gets a full crates.io redirect — every `cargo build` resolves via Artifactory:
//
//	[registry]
//	default = "jfrog"
//	[registries.jfrog]
//	index = "sparse+https://<host>/artifactory/api/cargo/<repo>/index/"
//	[source.crates-io]
//	replace-with = "jfrog"
//
// ~/.cargo/credentials.toml gets the token:
//
//	[registries.jfrog]
//	token = "Bearer <token>"
//
// deployRepo is optional: when non-empty (a local repo) it is written as a second registry
// ([registries.jfrog-local]) so `cargo publish --registry jfrog-local` uploads to it; resolution
// still goes through resolveRepo (a remote) via the crates.io redirect. When deployRepo is empty,
// only the resolution registry is configured (single-registry mode, e.g. when `--repo` is given).
//
// Existing unrelated keys in both files are preserved. Re-running is idempotent.
func ConfigureNativeRegistry(serverDetails *config.ServerDetails, resolveRepo, deployRepo string) error {
	if serverDetails == nil {
		return errorutils.CheckErrorf("server details are required to configure cargo")
	}
	resolveIndex, err := cargoSparseIndexURL(serverDetails.ArtifactoryUrl, resolveRepo)
	if err != nil {
		return err
	}
	var deployIndex string
	if deployRepo != "" {
		if deployIndex, err = cargoSparseIndexURL(serverDetails.ArtifactoryUrl, deployRepo); err != nil {
			return err
		}
	}
	home, err := cargoHome()
	if err != nil {
		return err
	}

	// 1. config.toml — resolution registry + full crates.io redirect + credential provider, plus an
	// optional deploy registry. The cargo:token provider is required for cargo to send the token from
	// credentials.toml; without it cargo errors "authenticated registries require a credential-provider".
	//
	// Re-running setup must produce a config that reflects only this invocation: if the user
	// previously configured a project-scoped jfrog-local registry (e.g. `jf setup cargo --project=X`)
	// and now re-runs with a --repo-only, no-deploy setup, the previously-written jfrog-local entry
	// must be deleted rather than left behind pointing at the old project's local repo. Uday's report:
	// "jfrog-local still project-scoped" after switching flows was exactly this stale-entry case.
	configPath := filepath.Join(home, "config.toml")
	if err = mergeTomlFile(configPath, 0644, func(m map[string]interface{}) {
		setNested(m, []string{"registry", "default"}, jfrogRegistryName)
		setNested(m, []string{"registry", "global-credential-providers"}, []string{"cargo:token"})
		setNested(m, []string{"registries", jfrogRegistryName, "index"}, resolveIndex)
		setNested(m, []string{"source", "crates-io", "replace-with"}, jfrogRegistryName)
		if deployIndex != "" {
			setNested(m, []string{"registries", jfrogDeployRegistryName, "index"}, deployIndex)
		} else {
			// No deploy repo this run — clear any stale jfrog-local from a prior setup.
			deleteNested(m, []string{"registries", jfrogDeployRegistryName})
		}
	}); err != nil {
		return fmt.Errorf("failed to write cargo config %q: %w", configPath, err)
	}

	// 2. credentials.toml — the credential for every configured registry (skip for anonymous access
	// when none is configured). Supports access-token ("Bearer …") and basic ("Basic base64(user:password)") auth.
	credential := cargoCredential(serverDetails)
	if credential == "" {
		log.Info("cargo: no access token or user/password in server config; configured registry for anonymous access")
		return nil
	}
	registries := []string{jfrogRegistryName}
	if deployRepo != "" {
		registries = append(registries, jfrogDeployRegistryName)
	}
	credsPath := filepath.Join(home, "credentials.toml")
	// 0600 up-front — credentials.toml holds a bearer token. A post-write chmod
	// leaves a world-readable window between create and chmod.
	if err = mergeTomlFile(credsPath, 0600, func(m map[string]interface{}) {
		for _, reg := range registries {
			setNested(m, []string{"registries", reg, "token"}, credential)
		}
		if deployRepo == "" {
			// Mirror the config.toml cleanup: drop any stale jfrog-local credentials so the two
			// files stay in sync (a leftover token here would otherwise reference a registry
			// that no longer exists in config.toml).
			deleteNested(m, []string{"registries", jfrogDeployRegistryName})
		}
	}); err != nil {
		return fmt.Errorf("failed to write cargo credentials %q: %w", credsPath, err)
	}
	return nil
}

// mergeTomlFile decodes an existing TOML file (treated as empty if missing) into a map, applies
// apply, then writes it back — creating parent directories as needed. Unrelated keys are
// preserved; comments and original key ordering are not.
//
// The write is atomic and never leaves a partial file at path: apply's output is encoded into a
// buffer first, then written to a temp file in the same directory with the requested perm, then
// renamed over path. Callers should pass 0600 for credential files and 0644 for regular config.
func mergeTomlFile(path string, perm os.FileMode, apply func(map[string]interface{})) error {
	m := map[string]interface{}{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil && len(data) > 0:
		if uerr := toml.Unmarshal(data, &m); uerr != nil {
			return fmt.Errorf("parse existing TOML: %w", uerr)
		}
	case err != nil && !os.IsNotExist(err):
		return err
	}

	apply(m)

	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Encode to a buffer first so a mid-encode failure never truncates path.
	var buf bytes.Buffer
	if err = toml.NewEncoder(&buf).Encode(m); err != nil {
		return fmt.Errorf("encode TOML: %w", err)
	}
	// Temp file in the same directory + rename → atomic replace with the intended perm from
	// the outset. Same-directory rename keeps atomicity on all supported filesystems.
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err = tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err = tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// setNested sets m[keys[0]][keys[1]]...=value, creating intermediate TOML tables
// (map[string]interface{}). value may be any TOML-encodable type (string, []string, ...).
// If an intermediate key already holds a non-table value, it is replaced with a fresh table so
// the write can proceed.
func setNested(m map[string]interface{}, keys []string, value interface{}) {
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = value
}

// deleteNested removes m[keys[0]][keys[1]]... if it exists. Intermediate tables that become
// empty as a result are also removed, so a stale [registries.jfrog-local] entry does not leave
// behind an empty [registries] block that toml would still emit. Missing keys are a no-op.
func deleteNested(m map[string]interface{}, keys []string) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		delete(m, keys[0])
		return
	}
	next, ok := m[keys[0]].(map[string]interface{})
	if !ok {
		return
	}
	deleteNested(next, keys[1:])
	if len(next) == 0 {
		delete(m, keys[0])
	}
}
