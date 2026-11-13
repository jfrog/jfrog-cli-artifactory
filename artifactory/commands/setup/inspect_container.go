package setup

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
			applyHelperLogin(&result, helper, serverHost, serverDetails)
			return result, nil
		}
	}
	key, ok := containerAuthKey(authFile, serverHost, serverDetails)
	if !ok {
		// helm registry login keeps the login only in the native credential store it
		// detects (osxkeychain, wincred, ...) and writes no auths entry.
		if authFile.CredsStore != "" {
			applyHelperLogin(&result, authFile.CredsStore, serverHost, serverDetails)
		}
		return result, nil
	}
	entry := authFile.Auths[key]
	result.status.State, result.status.Host = StateConfigured, serverHost
	switch {
	case entry.Auth != "":
		result.status.Credentials = CredentialsPresent
		result.credentials = decodeContainerAuth(entry.Auth)
	case entry.IdentityToken != "", authFile.CredsStore != "":
		// The secret lives in a credential helper; the entry records the login.
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

// applyHelperLogin marks the server's registry configured when the credential helper holds
// a login for it. It runs the helper's list action, which returns server URLs and user
// names but never secrets, and does not prompt.
func applyHelperLogin(result *inspection, helper, serverHost string, serverDetails *config.ServerDetails) {
	if strings.ContainsAny(helper, `/\`) {
		return
	}
	out, err := runTool("docker-credential-"+helper, "list")
	if err != nil {
		return
	}
	var logins map[string]string
	if json.Unmarshal(out, &logins) != nil {
		return
	}
	for serverURL := range logins {
		if matchServerHost(serverURL, serverDetails) {
			result.status.State, result.status.Host, result.status.Credentials = StateConfigured, serverHost, CredentialsPresent
			return
		}
	}
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

// podmanAuthFiles returns the files Podman searches for a registry's login, in its own
// order (containers/image getAuthFilePaths): REGISTRY_AUTH_FILE alone when set; otherwise
// the primary auth file `podman login` writes, then $XDG_CONFIG_HOME/containers/auth.json,
// then Docker's config.json. The legacy ~/.dockercfg, in an older format, is not read.
func podmanAuthFiles(goos string) ([]string, error) {
	if custom := os.Getenv("REGISTRY_AUTH_FILE"); custom != "" {
		return []string{filepath.Clean(custom)}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	var primary string
	switch {
	case goos != "linux":
		primary = filepath.Join(home, ".config", "containers", "auth.json")
	case os.Getenv("XDG_RUNTIME_DIR") != "":
		primary = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "auth.json")
	default:
		primary = filepath.Join("/run", "containers", strconv.Itoa(os.Getuid()), "auth.json")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	dockerConfig := os.Getenv("DOCKER_CONFIG")
	if dockerConfig == "" {
		dockerConfig = filepath.Join(home, ".docker")
	}
	files := []string{primary}
	for _, candidate := range []string{filepath.Join(configHome, "containers", "auth.json"), filepath.Join(dockerConfig, "config.json")} {
		if !slices.Contains(files, candidate) {
			files = append(files, candidate)
		}
	}
	return files, nil
}

// inspectPodman reports the first file, in Podman's search order, that holds a login for
// the server's registry. When none does, it reports the primary file `podman login` writes.
func inspectPodman(serverDetails *config.ServerDetails) (inspection, error) {
	files, err := podmanAuthFiles(runtime.GOOS)
	if err != nil {
		return inspection{}, err
	}
	found := binaryFound("podman")
	var primary inspection
	for i, path := range files {
		result, err := inspectContainerAuth(path, found, serverDetails)
		if err != nil {
			return inspection{}, err
		}
		if result.status.State == StateConfigured {
			return result, nil
		}
		if i == 0 {
			primary = result
		}
	}
	return primary, nil
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
