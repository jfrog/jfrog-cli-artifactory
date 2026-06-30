package cargo

import "strings"

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
