package cargo

import (
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

// cargoAuthToken returns the credential cargo should send to Artifactory: the access token if
// present, otherwise the password. Empty means "run/configure unauthenticated". Shared by the
// per-run env-var path (resolveAuthEnv) and the persistent `jf setup cargo` path.
func cargoAuthToken(sd *config.ServerDetails) string {
	if sd == nil {
		return ""
	}
	if sd.AccessToken != "" {
		return sd.AccessToken
	}
	return sd.Password
}

// cargoTokenValue formats a token the way cargo forwards it in the Authorization header.
// Both the env-var (CARGO_REGISTRIES_<NAME>_TOKEN) and credentials.toml paths use this.
//
// cargo's cargo:token provider sends the token value verbatim as the Authorization header, and
// Artifactory's Cargo index requires the "Bearer " scheme with a JFrog ACCESS TOKEN — verified
// live against ecosysjfrog: `Authorization: Bearer <access-token>` → 200, bare token → 401.
// (An API key, even with "Bearer ", is rejected as "Props Authentication Token not found" — the
// credential must be an access token, not an API key.)
func cargoTokenValue(token string) string {
	return "Bearer " + token
}

// commandBucket returns the build-info collection bucket for a cargo sub-command.
// "build" and "package" are intentionally NOT collected: they produce no build-info we want
// ("build" compiles with no publishable artifact; "package" only stages a local .crate). The
// artifact-producing operation we record is "publish" (the crate ends up in Artifactory).
// Both still authenticate (see needsRemoteAccess) so they resolve dependencies from Artifactory.
func commandBucket(cmd string) string {
	switch cmd {
	case "install", "update", "add", "fetch", "generate-lockfile", "run", "test", "check":
		return "deps"
	case "publish":
		return "publish"
	default:
		return "none"
	}
}

// needsRemoteAccess reports whether the command talks to the registry (and thus needs auth).
// "build" and "package" resolve dependencies from Artifactory (compile / verify build) even though
// we collect no build-info for them, so they are included despite bucketing to "none".
func needsRemoteAccess(cmd string) bool {
	switch cmd {
	case "build", "package":
		return true
	}
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

// buildAuthEnv returns the env entries injecting the registry token (Bearer form).
func buildAuthEnv(registryName, token string) []string {
	if registryName == "" || token == "" {
		return nil
	}
	return []string{cargoRegistryEnvKey(registryName) + "=" + cargoTokenValue(token)}
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
	token := cargoAuthToken(c.serverDetails)
	if token == "" {
		log.Debug("cargo: no token/password in server config; running unauthenticated")
		return nil
	}

	var env []string
	matched := map[string]bool{}
	for name, indexURL := range parseCargoRegistries(c.workingDir) {
		if registryHostMatches(indexURL, c.serverDetails.ArtifactoryUrl) {
			env = append(env, buildAuthEnv(name, token)...)
			matched[name] = true
		}
	}

	// Fallback: ensure the explicitly-named --registry is authenticated even if
	// config discovery missed it (e.g. registry configured outside the project dir).
	if regName := registryNameFromArgs(c.args); regName != "" && !matched[regName] {
		env = append(env, buildAuthEnv(regName, token)...)
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
