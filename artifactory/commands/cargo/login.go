package cargo

import (
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

// resolveAuthEnv builds the cargo registry token env from the jf server config.
// Returns nil (run unauthenticated) on any missing piece — never hard-fails.
func (c *CargoCommand) resolveAuthEnv() []string {
	regName := registryNameFromArgs(c.args)
	if regName == "" {
		log.Debug("cargo: no --registry in args; running unauthenticated")
		return nil
	}
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
	return buildAuthEnv(regName, token)
}
