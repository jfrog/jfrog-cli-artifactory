package apt

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var keyringsDir = "/etc/apt/keyrings"

// WriteTempSourcesList creates a temporary apt sources.list file in $TMPDIR
// containing ONLY the JFrog Artifactory entry, credentials embedded in the URL.
// Returns the path to the temp file.
//
// Used with: apt-get -o Dir::Etc::sourcelist=<path> -o Dir::Etc::sourceparts=- ...
// which replaces the main sources.list AND disables sources.list.d/, so this file
// is the sole active source. Packages therefore resolve exclusively through
// Artifactory — no other configured repository can serve them. This assumes the
// target repo is (or proxies) a complete apt source so dependencies resolve.
//
// Caller must defer os.Remove(path) to clean up.
func WriteTempSourcesList(serverDetails *config.ServerDetails, repoName, dist, component string, trusted bool) (string, error) {
	jfrogLine, err := buildSourcesLine(serverDetails, repoName, dist, component, trusted, "")
	if err != nil {
		return "", err
	}

	content := jfrogLine + "\n"

	f, err := os.CreateTemp("", "jfrog-apt-sources-*.list")
	if err != nil {
		return "", fmt.Errorf("create temp sources list: %w", err)
	}
	if err = f.Chmod(0600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("chmod temp sources list: %w", err)
	}
	if _, err = f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write temp sources list: %w", err)
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close temp sources list: %w", err)
	}
	return f.Name(), nil
}

// FetchAndInstallPublicKey downloads the GPG public key for the given Artifactory
// Debian repository and writes it to /etc/apt/keyrings/jfrog-<repo>-<dist>.asc.
// Returns the keyring path.
//
// The dist suffix in the filename ensures per-dist isolation: removing noble
// entries does not affect jammy keys, and vice versa.
//
// Auto-detects the signing key: queries the repo config for primaryKeyPairRef,
// fetches that named key if set, falls back to the default key otherwise.
// Requires root. The .asc extension tells apt the key is ASCII-armored — no
// gpg --dearmor step needed (supported since apt 1.4 / Ubuntu 22.04+).
func FetchAndInstallPublicKey(serverDetails *config.ServerDetails, repoName, dist string) (string, error) {
	sm, err := artutils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return "", fmt.Errorf("create service manager: %w", err)
	}

	var repoDetails struct {
		PrimaryKeyPairRef string `json:"primaryKeyPairRef"`
	}
	if err = sm.GetRepository(repoName, &repoDetails); err != nil {
		log.Debug("Could not determine repo signing key name, falling back to default: " + err.Error())
	}

	artURL := strings.TrimSuffix(serverDetails.GetArtifactoryUrl(), "/")
	var keyURL string
	if repoDetails.PrimaryKeyPairRef != "" {
		keyURL = fmt.Sprintf("%s/api/security/keypair/%s/public", artURL, repoDetails.PrimaryKeyPairRef)
		log.Debug(fmt.Sprintf("Using signing key '%s' for repository '%s'", repoDetails.PrimaryKeyPairRef, repoName))
	} else {
		keyURL = artURL + "/api/gpg/key/public"
		log.Debug("Using default Artifactory GPG public key")
	}

	httpClientDetails := sm.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	resp, body, _, err := sm.Client().SendGet(keyURL, true, &httpClientDetails)
	if err != nil {
		return "", fmt.Errorf("fetch public key: request failed: %w", err)
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		return "", fmt.Errorf("fetch public key: %w", err)
	}

	if err := os.MkdirAll(keyringsDir, 0755); err != nil {
		return "", fmt.Errorf("create keyrings dir: %w", err)
	}

	keyPath := filepath.Join(keyringsDir, fmt.Sprintf("jfrog-%s-%s.asc", repoName, dist))
	if err := os.WriteFile(keyPath, body, 0644); err != nil {
		return "", fmt.Errorf("write public key: %w", err)
	}
	return keyPath, nil
}

// buildSourcesLine returns a single deb sources.list line with credentials embedded in the URL.
//   - trusted=true  → [trusted=yes]           skip GPG verification (testing only)
//   - signedBy != "" → [signed-by=<path>]     scope trust to a specific keyring (prod)
//   - both empty    → no options              (requires repo is already signed + trusted system-wide)
func buildSourcesLine(serverDetails *config.ServerDetails, repoName, dist, component string, trusted bool, signedBy string) (string, error) {
	if err := validateSourcesToken("dist", dist); err != nil {
		return "", err
	}
	if err := validateSourcesToken("component", component); err != nil {
		return "", err
	}
	if err := validateSourcesToken("repo", repoName); err != nil {
		return "", err
	}

	user, password, err := serverDetails.GetAuthenticationCredentials()
	if err != nil {
		return "", err
	}

	artURL := strings.TrimSuffix(serverDetails.GetArtifactoryUrl(), "/")
	repoPath := artURL + "/" + repoName

	parsed, err := url.Parse(repoPath)
	if err != nil {
		return "", fmt.Errorf("parse artifactory URL: %w", err)
	}
	parsed.User = url.UserPassword(user, password)

	options := ""
	switch {
	case signedBy != "":
		options = fmt.Sprintf("[signed-by=%s] ", signedBy)
	case trusted:
		options = "[trusted=yes] "
	}
	return fmt.Sprintf("deb %s%s %s %s", options, parsed.String(), dist, component), nil
}

// validateSourcesToken rejects values that would corrupt a sources.list line.
// Newlines, carriage returns or null bytes in any token would inject extra lines.
func validateSourcesToken(field, value string) error {
	if value == "" {
		return fmt.Errorf("--%s must not be empty", field)
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\000' {
			return fmt.Errorf("invalid character in --%s: control characters are not allowed", field)
		}
	}
	return nil
}
