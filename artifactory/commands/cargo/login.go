package cargo

import (
	"encoding/base64"
	"net/url"
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// cargoCredentialProviderEnv is the env var controlling cargo's credential providers.
// Cargo (>=1.74) will not use an injected CARGO_REGISTRIES_<NAME>_TOKEN unless a provider is
// enabled; the built-in default is cargo:token, but a user config that omits it (or has none)
// makes cargo error with "authenticated registries require a credential-provider". We enable
// cargo:token for the child process so the injected token is actually consumed.
const cargoCredentialProviderEnv = "CARGO_REGISTRY_GLOBAL_CREDENTIAL_PROVIDERS"

// cargoCredential returns the exact Authorization-header value cargo's cargo:token provider should
// forward to Artifactory for the given server, selecting the auth scheme like the other package
// managers' setup flows do (prefer an access token, else fall back to basic auth):
//
//   - access token configured           -> "Bearer <access-token>"
//   - user + password (no access token) -> "Basic <base64(user:password)>"
//   - neither                            -> "" (anonymous)
//
// cargo forwards this value verbatim as the Authorization header. Artifactory's Cargo index accepts
// either scheme; the "Bearer" form must carry a JFrog ACCESS TOKEN (verified live: Bearer
// access-token → 200, bare token → 401), and the "Basic" form carries username:password.
// Shared by the per-run env-var path (resolveAuthEnv) and the persistent `jf setup cargo` path.
func cargoCredential(sd *config.ServerDetails) string {
	if sd == nil {
		return ""
	}
	if sd.AccessToken != "" {
		return "Bearer " + sd.AccessToken
	}
	if sd.User != "" && sd.Password != "" {
		enc := base64.StdEncoding.EncodeToString([]byte(sd.User + ":" + sd.Password))
		return "Basic " + enc
	}
	return ""
}

// commandBucket returns the build-info collection bucket for a cargo sub-command.
// Three cargo commands collect build-info:
//   - "install" -> "deps"    (records the resolved dependency graph)
//   - "build"   -> "deps"    (same shape — cargo build resolves + compiles, so the dep set is
//     exactly what was linked into the produced binaries; requested by users
//     because it is the everyday command, unlike install/publish which are
//     narrower flows)
//   - "publish" -> "publish" (records deps + the uploaded .crate artifact)
//
// Every other cargo command (fetch, update, add, check, test, run, package, metadata, …) is a
// pure pass-through: it bucket-maps to "none" and collects nothing. Authentication for those is
// expected to come from the user's own cargo credentials (e.g. written by `jf setup cargo`), not
// from per-run token injection.
func commandBucket(cmd string) string {
	switch cmd {
	case "install", "build":
		return "deps"
	case "publish":
		return "publish"
	default:
		return "none"
	}
}

// needsRemoteAccess reports whether jf should inject registry auth for the command. Only the two
// build-info-collecting commands (install, publish) are jf-integrated and get token injection so
// they can resolve/upload against Artifactory. All other commands are pass-throughs and rely on the
// user's cargo credentials (e.g. from `jf setup cargo`).
func needsRemoteAccess(cmd string) bool {
	switch commandBucket(cmd) {
	case "deps", "publish":
		return true
	default:
		return false
	}
}

// cargoRegistryEnvKey builds cargo's per-registry token env var name.
// Cargo uppercases the registry name and replaces '-' with '_'.
func cargoRegistryEnvKey(registryName string) string {
	norm := strings.ToUpper(strings.ReplaceAll(registryName, "-", "_"))
	return "CARGO_REGISTRIES_" + norm + "_TOKEN"
}

// buildAuthEnv returns the env entries injecting the registry credential. The credential is the
// full Authorization-header value ("Bearer <token>" or "Basic <base64>") produced by cargoCredential
// and forwarded verbatim by cargo's cargo:token provider.
func buildAuthEnv(registryName, credential string) []string {
	if registryName == "" || credential == "" {
		return nil
	}
	return []string{cargoRegistryEnvKey(registryName) + "=" + credential}
}

// registryNameFromArgs extracts the value of --registry (space or = form).
func registryNameFromArgs(args []string) string {
	for i, a := range args {
		if a == "--registry" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--registry=") {
			return strings.TrimPrefix(a, "--registry=")
		}
	}
	return ""
}

// registryHostMatches reports whether a cargo registry index URL points at the same
// host as the configured Artifactory server URL. Strips cargo's "sparse+"/"git+" prefixes.
func registryHostMatches(indexURL, artifactoryURL string) bool {
	strip := func(s string) string {
		s = strings.TrimPrefix(s, "sparse+")
		s = strings.TrimPrefix(s, "git+")
		return s
	}
	iu, err := url.Parse(strip(indexURL))
	if err != nil || iu.Host == "" {
		return false
	}
	au, err := url.Parse(artifactoryURL)
	if err != nil || au.Host == "" {
		return false
	}
	return strings.EqualFold(iu.Host, au.Host)
}

// resolveAuthEnv builds cargo registry token env vars for every registry in
// .cargo/config.toml whose index URL points at the configured JFrog server.
// Falls back to the --registry arg registry when config discovery yields nothing.
// Returns nil (run unauthenticated) on any missing piece — never hard-fails.
func (c *CargoCommand) resolveAuthEnv() []string {
	if c.serverDetails == nil {
		log.Debug("cargo: no server details; running unauthenticated")
		return nil
	}
	credential := cargoCredential(c.serverDetails)
	if credential == "" {
		log.Debug("cargo: no access token or user/password in server config; running unauthenticated")
		return nil
	}

	var env []string
	matched := map[string]bool{}
	for name, indexURL := range parseCargoRegistries(c.workingDir) {
		if registryHostMatches(indexURL, c.serverDetails.ArtifactoryUrl) {
			env = append(env, buildAuthEnv(name, credential)...)
			matched[name] = true
		}
	}

	// Fallback: ensure the explicitly-named --registry is authenticated even if
	// config discovery missed it (e.g. registry configured outside the project dir).
	if regName := registryNameFromArgs(c.args); regName != "" && !matched[regName] {
		env = append(env, buildAuthEnv(regName, credential)...)
	}

	if len(env) == 0 {
		log.Debug("cargo: no registries matched the configured server; running unauthenticated")
		return env
	}
	// We injected at least one token — ensure cargo has a credential provider to consume it.
	// Respect an explicit user override in the environment; otherwise enable cargo:token.
	if os.Getenv(cargoCredentialProviderEnv) == "" {
		env = append(env, cargoCredentialProviderEnv+"=cargo:token")
	}
	return env
}
