package setup

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// containerAuthFile is the credential file format Docker, Podman and Helm share.
type containerAuthFile struct {
	Auths map[string]struct {
		Auth          string `json:"auth"`
		IdentityToken string `json:"identitytoken"`
	} `json:"auths"`
	CredsStore  string            `json:"credsStore"`
	CredHelpers map[string]string `json:"credHelpers"`
}

// inspectContainerAuth reports whether the auth file holds a login for the server's
// registry. These tools keep logins for many registries side by side, so another
// registry's login is normal and never reported as other-host.
func inspectContainerAuth(path string, found bool, serverDetails *config.ServerDetails) (inspection, error) {
	result := newInspection(found)
	result.status.Location = path
	serverHost, err := deriveContainerRegistryHost(serverDetails)
	if err != nil {
		return inspection{}, err
	}
	content, exists, err := readOptionalFile(path)
	if err != nil || !exists || len(strings.TrimSpace(string(content))) == 0 {
		return result, err
	}
	var authFile containerAuthFile
	if err = json.Unmarshal(content, &authFile); err != nil {
		return inspection{}, errorutils.CheckErrorf("failed to parse %s: %s", path, err.Error())
	}
	for key, helper := range authFile.CredHelpers {
		if helper != "" && matchServerHost(key, serverDetails) {
			result.status.State, result.status.Host, result.status.Credentials = StateConfigured, serverHost, CredentialsPresent
			return result, nil
		}
	}
	key, ok := containerAuthKey(authFile, serverHost, serverDetails)
	if !ok {
		return result, nil
	}
	entry := authFile.Auths[key]
	result.status.State, result.status.Host = StateConfigured, serverHost
	switch {
	case entry.Auth != "":
		result.status.Credentials = CredentialsPresent
		result.credentials = decodeContainerAuth(entry.Auth)
	case entry.IdentityToken != "", authFile.CredsStore != "":
		// The secret lives in a credential helper, which status does not run.
		result.status.Credentials = CredentialsPresent
	default:
		result.status.Credentials = CredentialsAbsent
	}
	return result, nil
}

// containerAuthKey picks the auths entry for the server's registry the way Docker does:
// the bare host first. Other spellings of the same host ("https://host/v1/") are tried in
// sorted order, preferring one that holds a secret, so the answer never depends on map order.
func containerAuthKey(authFile containerAuthFile, serverHost string, serverDetails *config.ServerDetails) (string, bool) {
	if _, ok := authFile.Auths[serverHost]; ok {
		return serverHost, true
	}
	var matching []string
	for key := range authFile.Auths {
		if matchServerHost(key, serverDetails) {
			matching = append(matching, key)
		}
	}
	if len(matching) == 0 {
		return "", false
	}
	slices.Sort(matching)
	for _, key := range matching {
		if entry := authFile.Auths[key]; entry.Auth != "" || entry.IdentityToken != "" {
			return key, true
		}
	}
	return matching[0], true
}

func decodeContainerAuth(encoded string) storedCredentials {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return storedCredentials{}
	}
	user, password, _ := strings.Cut(string(decoded), ":")
	return storedCredentials{user: user, password: password}
}

func inspectDocker(serverDetails *config.ServerDetails) (inspection, error) {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return inspection{}, errorutils.CheckError(err)
		}
		configDir = filepath.Join(home, ".docker")
	}
	return inspectContainerAuth(filepath.Join(configDir, "config.json"), binaryFound("docker"), serverDetails)
}

// podmanAuthFile applies Podman's lookup order: REGISTRY_AUTH_FILE, then the runtime
// directory on Linux, then the per-user config directory.
func podmanAuthFile() (string, error) {
	if custom := os.Getenv("REGISTRY_AUTH_FILE"); custom != "" {
		return filepath.Clean(custom), nil
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtime.GOOS == "linux" && runtimeDir != "" {
		candidate := filepath.Join(runtimeDir, "containers", "auth.json")
		// #nosec G703 -- Podman's own auth file under XDG_RUNTIME_DIR; only its existence is checked
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errorutils.CheckError(err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "containers", "auth.json"), nil
}

func inspectPodman(serverDetails *config.ServerDetails) (inspection, error) {
	path, err := podmanAuthFile()
	if err != nil {
		return inspection{}, err
	}
	return inspectContainerAuth(path, binaryFound("podman"), serverDetails)
}

// helmRegistryConfig returns the registry config `helm registry login` writes:
// HELM_REGISTRY_CONFIG, else what helm reports, else helm's own lookup (HELM_CONFIG_HOME,
// then XDG_CONFIG_HOME, then the platform default).
func helmRegistryConfig(helmFound bool) (string, error) {
	if custom := os.Getenv("HELM_REGISTRY_CONFIG"); custom != "" {
		return filepath.Clean(custom), nil
	}
	if helmFound {
		if out, err := runTool("helm", "env", "HELM_REGISTRY_CONFIG"); err == nil {
			if path := strings.TrimSpace(string(out)); path != "" {
				return path, nil
			}
		}
	}
	if helmHome := os.Getenv("HELM_CONFIG_HOME"); helmHome != "" {
		return filepath.Join(helmHome, "registry", "config.json"), nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		switch runtime.GOOS {
		case "windows":
			configHome = os.Getenv("APPDATA")
		case "darwin":
			home, err := os.UserHomeDir()
			if err != nil {
				return "", errorutils.CheckError(err)
			}
			configHome = filepath.Join(home, "Library", "Preferences")
		default:
			home, err := os.UserHomeDir()
			if err != nil {
				return "", errorutils.CheckError(err)
			}
			configHome = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(configHome, "helm", "registry", "config.json"), nil
}

func inspectHelm(serverDetails *config.ServerDetails) (inspection, error) {
	found := binaryFound("helm")
	path, err := helmRegistryConfig(found)
	if err != nil {
		return inspection{}, err
	}
	return inspectContainerAuth(path, found, serverDetails)
}
