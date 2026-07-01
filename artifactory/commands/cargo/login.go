package cargo

import (
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

func commandBucket(cmd string) string {
	switch cmd {
	case "build", "install", "update", "add", "fetch", "generate-lockfile", "run", "test", "check":
		return "deps"
	case "package":
		return "artifacts"
	case "publish":
		return "publish"
	default:
		return "none"
	}
}

// needsRemoteAccess reports whether the command talks to the registry (and thus needs auth).
func needsRemoteAccess(cmd string) bool {
	switch commandBucket(cmd) {
	case "deps", "artifacts", "publish":
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
	return []string{cargoRegistryEnvKey(registryName) + "=Bearer " + token}
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
	token := c.serverDetails.AccessToken
	if token == "" {
		token = c.serverDetails.Password
	}
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
	}
	return env
}
