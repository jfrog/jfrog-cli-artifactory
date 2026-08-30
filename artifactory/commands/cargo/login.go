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

// cargo per-registry token env vars follow the shape CARGO_REGISTRIES_<NAME>_TOKEN, where the
// name is uppercased and '-' is replaced with '_'. The two consts pin the prefix and suffix
// cargo actually looks for so a rename of the env-var scheme is a single edit-point.
const (
	cargoRegistryTokenEnvPrefix = "CARGO_REGISTRIES_"
	cargoRegistryTokenEnvSuffix = "_TOKEN"
	cargoCredentialProviderName = "cargo:token"
)

// Authorization-header schemes cargo forwards verbatim to the registry. Kept as consts so the
// exact wire format (mandated by Artifactory's Cargo index) is not scattered as free strings.
const (
	authSchemeBearer = "Bearer "
	authSchemeBasic  = "Basic "
)

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
//
// Precedence — access token > user+password — is deliberate and matches every other package-manager
// setup flow in this codebase (nix, npm, pip, twine, gradle, maven, conan, huggingface, helm all
// prefer AccessToken when present). Naveen raised the concern that "user's command flags are not
// considered if some token exists, that may be stale". The rationale for the current ordering:
//  1. `sd` here is the FINAL merged ServerDetails — flags have already been overlaid onto saved
//     config by the CLI layer, so a `--access-token` on the command line will land in sd.AccessToken
//     and be honoured. A "stale" token can only appear if it was saved by a prior
//     `jf c add` — in that case the user's fix is `jf c edit` or an explicit `--access-token=""`,
//     not a silent downgrade to basic auth here (which would surprise a user who intentionally set
//     both for two different purposes).
//  2. Access tokens are the JFrog-recommended auth for machine flows: they can be scoped, expired
//     and rotated per-service. Silently preferring the coarser user+password when both are set
//     would move users off the safer credential without warning them.
//  3. Consistency: reversing the order here alone would create a package-manager-specific quirk
//     (cargo would behave differently from every sibling command). If we ever decide to flip it,
//     that is a codebase-wide policy change — not a cargo-only carve-out.
func cargoCredential(sd *config.ServerDetails) string {
	if sd == nil {
		return ""
	}
	if sd.AccessToken != "" {
		return authSchemeBearer + sd.AccessToken
	}
	if sd.User != "" && sd.Password != "" {
		enc := base64.StdEncoding.EncodeToString([]byte(sd.User + ":" + sd.Password))
		return authSchemeBasic + enc
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

// needsRemoteAccess reports whether jf should inject registry auth (as CARGO_REGISTRIES_*_TOKEN
// env vars) for the command. There are two layers of authentication a cargo command can use:
//
//  1. Per-run env-var injection — what this function gates. jf reads the server details it
//     was configured with (`jf c add`) and, for the command being run, exports the matching
//     `CARGO_REGISTRIES_<NAME>_TOKEN` env var to the child cargo process. Enabled ONLY for
//     the three build-info-collecting commands (install, build, publish) because those are the
//     jf-integrated ones — jf actually needs to know the auth to attribute build info.
//
//  2. Persistent files — `~/.cargo/config.toml` + `~/.cargo/credentials.toml` written by
//     `jf setup cargo` (see ConfigureNativeRegistry). Once these exist, EVERY cargo invocation
//     — `cargo update`, `cargo search`, `cargo fetch`, `cargo add`, `cargo yank`, and yes,
//     even plain `cargo` outside of `jf cargo` — authenticates from those files. The pass-through
//     `jf cargo <cmd>` commands (that get `needsRemoteAccess == false`) do not skip auth: they
//     rely on this layer instead of the env-var layer.
//
// So the answer to "what about `jf cargo update` / `search` / `fetch`?" is: they authenticate
// exactly the same way the plain `cargo` binary does. If the user has run `jf setup cargo` (or
// otherwise has credentials in `~/.cargo/credentials.toml`), those commands work. If they haven't,
// those commands fail with cargo's own auth error — which is the expected UX ("please log in
// first") and matches how every other package-manager CLI in this repo behaves.
//
// We deliberately do NOT export `CARGO_REGISTRIES_*_TOKEN` for pass-through commands because:
//   - Doing so would silently override the user's `credentials.toml` (env wins over file), which
//     hides misconfiguration when they run the same command outside jf.
//   - Cargo's credential-provider gating means an injected token without also injecting
//     `CARGO_REGISTRY_GLOBAL_CREDENTIAL_PROVIDERS=cargo:token` is ignored anyway; adding both for
//     every pass-through would layer env noise on top of a config that already works.
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
	return cargoRegistryTokenEnvPrefix + norm + cargoRegistryTokenEnvSuffix
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

// registryHostMatches reports whether a cargo registry index URL points at the SAME Artifactory
// instance as the configured server URL — same host AND same base path. Strips cargo's
// "sparse+"/"git+" prefixes.
//
// Host-only matching would leak the Artifactory token to any other cargo registry hosted on the
// same host (a different service reverse-proxied on a different path, another Cargo index on the
// same domain, etc.). Requiring the registry URL to sit under the Artifactory base path scopes
// the token to Artifactory-served registries only.
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
	if !strings.EqualFold(iu.Host, au.Host) {
		return false
	}
	// Same host — require the registry path to sit under the Artifactory base path.
	// Normalise both to always end with "/" so "/artifactory" does not match "/artifactoryX".
	base := strings.TrimSuffix(au.Path, "/") + "/"
	regPath := strings.TrimSuffix(iu.Path, "/") + "/"
	if base == "/" {
		// Empty Artifactory base path (bare host) — host equality is the only signal available.
		return true
	}
	return strings.HasPrefix(regPath, base)
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
		env = append(env, cargoCredentialProviderEnv+"="+cargoCredentialProviderName)
	}
	return env
}
