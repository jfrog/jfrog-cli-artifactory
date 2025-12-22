// Package conan provides Conan package manager integration for JFrog Artifactory.
package conan

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	maxLoginAttempts = 2
)

// ConanRemote represents a Conan remote configuration.
type ConanRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ExtractRemoteName extracts the remote name from conan upload arguments.
// Looks for -r or --remote flag.
func ExtractRemoteName(args []string) string {
	for i, arg := range args {
		if arg == "-r" || arg == "--remote" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if strings.HasPrefix(arg, "-r=") {
			return strings.TrimPrefix(arg, "-r=")
		}
		if strings.HasPrefix(arg, "--remote=") {
			return strings.TrimPrefix(arg, "--remote=")
		}
	}
	return ""
}

// ValidateAndLogin validates the remote config exists and performs login.
// Returns the matched server details for artifact collection.
func ValidateAndLogin(remoteName string) (*config.ServerDetails, error) {
	// Get the remote URL from Conan
	remoteURL, err := getRemoteURL(remoteName)
	if err != nil {
		return nil, fmt.Errorf("get Conan remote URL: %w", err)
	}
	log.Debug(fmt.Sprintf("Conan remote '%s' URL: %s", remoteName, remoteURL))

	// Extract base Artifactory URL
	baseURL := extractBaseURL(remoteURL)
	log.Debug(fmt.Sprintf("Extracted base URL: %s", baseURL))

	// Find all matching JFrog CLI server configs
	matchingConfigs, err := findMatchingServers(baseURL)
	if err != nil {
		return nil, err
	}

	// Try to login with each matching config
	return tryLoginWithConfigs(remoteName, matchingConfigs)
}

// ListConanRemotes returns all configured Conan remotes.
func ListConanRemotes() ([]ConanRemote, error) {
	cmd := exec.Command("conan", "remote", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("conan remote list failed: %w", err)
	}

	var remotes []ConanRemote
	if err := json.Unmarshal(output, &remotes); err != nil {
		return nil, fmt.Errorf("parse conan remote list: %w", err)
	}

	return remotes, nil
}

// getRemoteURL retrieves the URL for a Conan remote.
func getRemoteURL(remoteName string) (string, error) {
	remotes, err := ListConanRemotes()
	if err != nil {
		return "", err
	}

	for _, remote := range remotes {
		if remote.Name == remoteName {
			return remote.URL, nil
		}
	}

	return "", fmt.Errorf("remote '%s' not found in Conan remotes", remoteName)
}

// ExtractRepoName extracts the Artifactory repository name from a Conan remote URL.
// Example: "https://myserver.jfrog.io/artifactory/api/conan/repo-name" -> "repo-name"
func ExtractRepoName(remoteURL string) string {
	// Format: .../api/conan/<repo-name>
	idx := strings.Index(remoteURL, "/api/conan/")
	if idx != -1 {
		repoName := remoteURL[idx+len("/api/conan/"):]
		// Remove trailing slash if present
		repoName = strings.TrimSuffix(repoName, "/")
		return repoName
	}
	return ""
}

// GetRepoNameForRemote gets the Artifactory repository name for a Conan remote.
func GetRepoNameForRemote(remoteName string) (string, error) {
	remoteURL, err := getRemoteURL(remoteName)
	if err != nil {
		return "", err
	}
	repoName := ExtractRepoName(remoteURL)
	if repoName == "" {
		return "", fmt.Errorf("could not extract repo name from URL: %s", remoteURL)
	}
	return repoName, nil
}

// extractBaseURL extracts the base Artifactory URL from a Conan remote URL.
// Example: "https://myserver.jfrog.io/artifactory/api/conan/repo" -> "https://myserver.jfrog.io"
func extractBaseURL(remoteURL string) string {
	// Find /artifactory/ and extract everything before it
	idx := strings.Index(remoteURL, "/artifactory/")
	if idx != -1 {
		return remoteURL[:idx]
	}
	idx = strings.Index(remoteURL, "/artifactory")
	if idx != -1 {
		return remoteURL[:idx]
	}
	return strings.TrimSuffix(remoteURL, "/")
}

// findMatchingServers finds all JFrog CLI server configs that match the remote URL.
func findMatchingServers(remoteBaseURL string) ([]*config.ServerDetails, error) {
	allConfigs, err := config.GetAllServersConfigs()
	if err != nil {
		return nil, fmt.Errorf("get JFrog CLI server configs: %w", err)
	}

	if len(allConfigs) == 0 {
		return nil, fmt.Errorf("no JFrog CLI server configurations found. Please run 'jf c add' to configure a server")
	}

	normalizedTarget := normalizeURL(remoteBaseURL)

	var matchingConfigs []*config.ServerDetails
	seenServerIDs := make(map[string]bool)

	for _, cfg := range allConfigs {
		if seenServerIDs[cfg.ServerId] {
			continue
		}

		if matchesServer(cfg, normalizedTarget) {
			matchingConfigs = append(matchingConfigs, cfg)
			seenServerIDs[cfg.ServerId] = true
		}
	}

	if len(matchingConfigs) == 0 {
		return nil, buildNoMatchError(remoteBaseURL, allConfigs)
	}

	return matchingConfigs, nil
}

// matchesServer checks if a server config matches the target URL.
func matchesServer(cfg *config.ServerDetails, normalizedTarget string) bool {
	// Check Artifactory URL
	if cfg.ArtifactoryUrl != "" {
		normalizedArt := normalizeURL(cfg.ArtifactoryUrl)
		normalizedArt = strings.TrimSuffix(normalizedArt, "/artifactory")
		if normalizedArt == normalizedTarget {
			return true
		}
	}

	// Check platform URL
	if cfg.Url != "" {
		normalizedPlatform := normalizeURL(cfg.Url)
		if normalizedPlatform == normalizedTarget {
			return true
		}
	}

	return false
}

// normalizeURL normalizes a URL for comparison.
func normalizeURL(u string) string {
	return strings.TrimSuffix(strings.ToLower(u), "/")
}

// buildNoMatchError creates a helpful error message when no matching config is found.
func buildNoMatchError(remoteBaseURL string, allConfigs []*config.ServerDetails) error {
	var configuredServers []string
	for _, cfg := range allConfigs {
		url := cfg.Url
		if url == "" {
			url = cfg.ArtifactoryUrl
		}
		if url != "" {
			configuredServers = append(configuredServers, fmt.Sprintf("  - %s: %s", cfg.ServerId, url))
		}
	}

	return fmt.Errorf(`no matching JFrog CLI server config found for remote URL: %s

The Conan remote points to an Artifactory instance that is not configured in JFrog CLI.
Please add the server configuration using: jf c add

Configured servers:
%s`, remoteBaseURL, strings.Join(configuredServers, "\n"))
}

// tryLoginWithConfigs attempts login with each matching config.
func tryLoginWithConfigs(remoteName string, configs []*config.ServerDetails) (*config.ServerDetails, error) {
	var allErrors []string

	for _, serverDetails := range configs {
		log.Debug(fmt.Sprintf("Trying to login with config '%s'...", serverDetails.ServerId))

		var lastErr error
		for attempt := 1; attempt <= maxLoginAttempts; attempt++ {
			if attempt > 1 {
				log.Debug(fmt.Sprintf("Retrying login with '%s' (attempt %d/%d)...", serverDetails.ServerId, attempt, maxLoginAttempts))
			}

			lastErr = loginToRemote(remoteName, serverDetails)
			if lastErr == nil {
				log.Info(fmt.Sprintf("Successfully logged into Conan remote '%s' using JFrog CLI config '%s'", remoteName, serverDetails.ServerId))
				return serverDetails, nil
			}

			log.Debug(fmt.Sprintf("Login attempt %d with '%s' failed: %s", attempt, serverDetails.ServerId, lastErr.Error()))
		}

		allErrors = append(allErrors, fmt.Sprintf("  - %s: %s", serverDetails.ServerId, lastErr.Error()))
	}

	return nil, fmt.Errorf(`failed to login to Conan remote '%s' with all matching JFrog CLI configs.

Tried the following configs:
%s

Please verify your credentials are correct. You can update them using: jf c add --interactive`, remoteName, strings.Join(allErrors, "\n"))
}

// loginToRemote logs into a Conan remote using JFrog CLI credentials.
func loginToRemote(remoteName string, serverDetails *config.ServerDetails) error {
	username, password, err := extractCredentials(serverDetails)
	if err != nil {
		return err
	}

	log.Debug(fmt.Sprintf("Logging into Conan remote '%s' using config '%s'", remoteName, serverDetails.ServerId))

	cmd := exec.Command("conan", "remote", "login", remoteName, username, "-p", password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		return fmt.Errorf("conan remote login failed: %s", outputStr)
	}

	return nil
}

// extractCredentials extracts username and password/token from server details.
// For Conan authentication with Artifactory
func extractCredentials(serverDetails *config.ServerDetails) (username, password string, err error) {
	username = serverDetails.User
	if username == "" {
		username = "admin"
	}

	// Prefer password over access token for Conan (API keys work more reliably)
	if serverDetails.Password != "" {
		password = serverDetails.Password
		return username, password, nil
	}

	// Fall back to access token if no password
	if serverDetails.AccessToken != "" {
		password = serverDetails.AccessToken
		return username, password, nil
	}

	return "", "", fmt.Errorf("no credentials (password or access token) found in JFrog CLI config for server '%s'", serverDetails.ServerId)
}

// formatServerIDs returns a comma-separated list of server IDs.
func formatServerIDs(configs []*config.ServerDetails) string {
	var ids []string
	for _, c := range configs {
		ids = append(ids, c.ServerId)
	}
	return strings.Join(ids, ", ")
}
