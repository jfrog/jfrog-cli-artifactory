package alpine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	apkKeysDir          = "/etc/apk/keys"
	apkRepositoriesFile = "/etc/apk/repositories"
)

// ApkConfigCommand downloads the Artifactory RSA public key and optionally writes it to disk.
// By default (apply == false) it prints the equivalent shell commands to stdout without touching
// any files. Pass --apply to actually write /etc/apk/keys/ and /etc/apk/repositories.
type ApkConfigCommand struct {
	commandName   string
	serverDetails *config.ServerDetails
	repoKey       string
	alpineVersion string
	branch        string
	username      string
	password      string
	apply         bool
}

// NewApkConfigCommand constructs an ApkConfigCommand.
func NewApkConfigCommand() *ApkConfigCommand {
	return &ApkConfigCommand{commandName: "apk-config"}
}

// SetServerDetails sets the Artifactory server config.
func (apkCmd *ApkConfigCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApkConfigCommand {
	apkCmd.serverDetails = serverDetails
	return apkCmd
}

// SetRepo sets the Artifactory Alpine repository key.
func (apkCmd *ApkConfigCommand) SetRepo(repoKey string) *ApkConfigCommand {
	apkCmd.repoKey = repoKey
	return apkCmd
}

// SetAlpineVersion sets the Alpine release tag (e.g. "v3.20").
func (apkCmd *ApkConfigCommand) SetAlpineVersion(version string) *ApkConfigCommand {
	apkCmd.alpineVersion = version
	return apkCmd
}

// SetBranch sets the Alpine repository branch (main, community, edge).
func (apkCmd *ApkConfigCommand) SetBranch(branch string) *ApkConfigCommand {
	apkCmd.branch = branch
	return apkCmd
}

// SetUsername sets the username CLI flag override.
func (apkCmd *ApkConfigCommand) SetUsername(username string) *ApkConfigCommand {
	apkCmd.username = username
	return apkCmd
}

// SetPassword sets the password CLI flag override.
func (apkCmd *ApkConfigCommand) SetPassword(password string) *ApkConfigCommand {
	apkCmd.password = password
	return apkCmd
}

// SetApply controls whether config is written to disk (true) or printed to stdout (false).
func (apkCmd *ApkConfigCommand) SetApply(apply bool) *ApkConfigCommand {
	apkCmd.apply = apply
	return apkCmd
}

// CommandName satisfies the Command interface.
func (apkCmd *ApkConfigCommand) CommandName() string {
	return apkCmd.commandName
}

// ServerDetails satisfies the Command interface.
func (apkCmd *ApkConfigCommand) ServerDetails() (*config.ServerDetails, error) {
	return apkCmd.serverDetails, nil
}

// Run downloads the RSA key, writes it to disk, and registers the Artifactory repo URL.
func (apkCmd *ApkConfigCommand) Run() error {
	if apkCmd.branch == "" {
		apkCmd.branch = "main"
	}

	if apkCmd.serverDetails == nil {
		return fmt.Errorf("no JFrog server configured — run 'jf c add' first or pass --server-id")
	}

	token := apkCmd.serverDetails.GetAccessToken()
	rtURL := strings.TrimRight(apkCmd.serverDetails.GetArtifactoryUrl(), "/")
	keyEndpoint := fmt.Sprintf("%s/api/security/keypair/public/repositories/%s", rtURL, apkCmd.repoKey)

	pemKey, err := downloadRSAKey(keyEndpoint, token)
	if err != nil {
		return err
	}

	keyPairRef, err := fetchKeyPairRef(rtURL, apkCmd.repoKey, token)
	if err != nil {
		log.Debug("Could not resolve key pair name from repo config, falling back to repo key:", err)
		keyPairRef = apkCmd.repoKey
	}

	repoURL := fmt.Sprintf("%s/%s/%s/%s/", rtURL, apkCmd.repoKey, apkCmd.alpineVersion, apkCmd.branch)
	keyFileName := keyPairRef + ".rsa.pub"
	keyFilePath := filepath.Join(apkKeysDir, keyFileName)

	if !apkCmd.apply {
		return apkCmd.printSetupScript(pemKey, keyFilePath, repoURL)
	}
	return apkCmd.applyConfig(pemKey, keyFilePath, repoURL)
}

// downloadRSAKey fetches the RSA public key from the Artifactory keypair API using Bearer auth.
func downloadRSAKey(endpoint, token string) (string, error) {
	if token == "" {
		return "", errorutils.CheckErrorf("no access token found — run 'jf c add' and configure an access token")
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", errorutils.CheckErrorf("failed to build RSA key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errorutils.CheckErrorf("failed to download RSA key from Artifactory: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errorutils.CheckErrorf("failed to read RSA key response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", errorutils.CheckErrorf("RSA key download failed with HTTP %d: %s", resp.StatusCode, string(body))
	}

	pem := string(body)
	if !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		return "", errorutils.CheckErrorf("Artifactory returned an unexpected response for the RSA key endpoint (does the repo have a signing keypair configured?)")
	}
	return pem, nil
}

// fetchKeyPairRef queries GET /api/repositories/<repo> and returns the primaryKeyPairRef value.
// This is the alias that Artifactory embeds in the APKINDEX signature, and must match the
// public key filename placed in /etc/apk/keys/ for APK to trust the repo.
func fetchKeyPairRef(rtURL, repoKey, token string) (string, error) {
	url := fmt.Sprintf("%s/api/repositories/%s", rtURL, repoKey)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errorutils.CheckErrorf("GET %s returned HTTP %d", url, resp.StatusCode)
	}

	var repoConfig struct {
		PrimaryKeyPairRef string `json:"primaryKeyPairRef"`
	}
	if err := json.Unmarshal(body, &repoConfig); err != nil {
		return "", err
	}
	if repoConfig.PrimaryKeyPairRef == "" {
		return "", errorutils.CheckErrorf("no primaryKeyPairRef configured on repo %q — attach a key pair first", repoKey)
	}
	return repoConfig.PrimaryKeyPairRef, nil
}

// printSetupScript prints the equivalent shell commands to stdout without writing any files.
// This is the default (no --apply) behaviour so users can review or pipe the output.
func (apkCmd *ApkConfigCommand) printSetupScript(pemKey, keyFilePath, repoURL string) error {
	fmt.Printf("# Run the following commands to configure APK to use your Artifactory repository:\n\n")
	fmt.Printf("mkdir -p %s\n", apkKeysDir)
	fmt.Printf("cat > %s << 'EOF'\n%sEOF\n\n", keyFilePath, pemKey)
	fmt.Printf("# Remove default Alpine CDN mirrors and add Artifactory:\n")
	fmt.Printf("grep -v 'dl-cdn.alpinelinux.org' /etc/apk/repositories > /tmp/repos.tmp && mv /tmp/repos.tmp /etc/apk/repositories\n")
	fmt.Printf("echo '%s' >> /etc/apk/repositories\n\n", repoURL)
	fmt.Printf("# Re-run with --apply to write these changes automatically:\n")
	fmt.Printf("# jf apk config --repo %s --alpine-version %s --branch %s --apply\n", apkCmd.repoKey, apkCmd.alpineVersion, apkCmd.branch)
	return nil
}

// applyConfig writes the RSA key to disk and appends the repo URL to /etc/apk/repositories.
func (apkCmd *ApkConfigCommand) applyConfig(pemKey, keyFilePath, repoURL string) error {
	if err := os.MkdirAll(apkKeysDir, 0755); err != nil {
		return errorutils.CheckErrorf("failed to create %s: %w", apkKeysDir, err)
	}
	if err := os.WriteFile(keyFilePath, []byte(pemKey), 0644); err != nil {
		return errorutils.CheckErrorf("failed to write RSA key to %s: %w", keyFilePath, err)
	}
	log.Info("RSA key written to", keyFilePath)

	existing, err := os.ReadFile(apkRepositoriesFile)
	if err != nil && !os.IsNotExist(err) {
		return errorutils.CheckErrorf("failed to read %s: %w", apkRepositoriesFile, err)
	}

	// Filter out any existing lines that point to the same host so we don't duplicate,
	// but keep third-party repos that are unrelated to this Artifactory instance.
	// Lines that originate from the default Alpine CDN mirrors are replaced entirely
	// so all traffic flows through Artifactory.
	var kept []string
	for _, line := range strings.Split(string(existing), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == repoURL {
			continue
		}
		// Drop default Alpine CDN mirror lines — Artifactory virtual repo proxies them.
		if strings.Contains(trimmed, "dl-cdn.alpinelinux.org") {
			continue
		}
		kept = append(kept, trimmed)
	}
	kept = append(kept, repoURL)

	content := strings.Join(kept, "\n") + "\n"
	// apkRepositoriesFile is a compile-time constant (/etc/apk/repositories), not user-controlled.
	if err := os.WriteFile(filepath.Clean(apkRepositoriesFile), []byte(content), 0644); err != nil { //nolint:gosec
		return errorutils.CheckErrorf("failed to write %s: %w", apkRepositoriesFile, err)
	}
	log.Info("Repository configured:", apkRepositoriesFile, "→", repoURL)
	return nil
}
