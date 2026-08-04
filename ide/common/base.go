package common

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// GetServerDetails resolves the server configuration to use.
func GetServerDetails(c *components.Context) (*config.ServerDetails, error) {
	// Case 1: explicit auth or server-id → flag-based path.
	if hasCredentialFlags(c) || c.IsFlagSet("server-id") {
		return pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	}

	// Case 2: --url alone → borrow credentials from the default server, but
	// only when --url points at the SAME host as that default server.
	// When the hosts differ we ask for explicit credentials instead.
	if c.IsFlagSet("url") {
		defaults, err := config.GetDefaultServerConf()
		if err != nil || defaults == nil {
			return nil, fmt.Errorf(
				"--url was provided but no default server is configured and no credentials were supplied. " +
					"Either run 'jf config add' to configure a server, or pass --access-token " +
					"(or --user and --password) alongside --url")
		}
		overrideUrl := c.GetStringFlagValue("url")
		if !sameHost(overrideUrl, defaults) {
			return nil, fmt.Errorf(
				"--url %q points at a different host than the configured default server (%q). "+
					"Either pass --access-token (or --user and --password) flags, "+
					"or use --server-id to use a configured jf config",
				overrideUrl, defaultServerHost(defaults))
		}
		defaults.ArtifactoryUrl = overrideUrl
		return defaults, nil
	}

	// Case 3: no flags → default server.
	rtDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("no default server configured")
	}
	if rtDetails == nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --url and --access-token flags")
	}
	if rtDetails.ArtifactoryUrl == "" && rtDetails.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}
	return rtDetails, nil
}

// hasCredentialFlags is true when the user supplied credentials on the
// command line (as opposed to only supplying --url or --server-id).
func hasCredentialFlags(c *components.Context) bool {
	return c.IsFlagSet("user") ||
		c.IsFlagSet("password") ||
		c.IsFlagSet("access-token")
}

// sameHost checks whether rawUrl targets the same host (host + port) as the
// default server. Missing or unparseable inputs are treated as a mismatch so
// credentials are never reused when we can't confirm the target.
func sameHost(rawUrl string, defaults *config.ServerDetails) bool {
	if rawUrl == "" || defaults == nil {
		return false
	}
	u, err := url.Parse(rawUrl)
	if err != nil || u.Host == "" {
		return false
	}
	defaultUrl := defaults.ArtifactoryUrl
	if defaultUrl == "" {
		defaultUrl = defaults.Url
	}
	d, err := url.Parse(defaultUrl)
	if err != nil || d.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, d.Host)
}

// defaultServerHost returns the host of the default server for use in error
// messages. Returns "" if it cannot be extracted.
func defaultServerHost(defaults *config.ServerDetails) string {
	if defaults == nil {
		return ""
	}
	raw := defaults.ArtifactoryUrl
	if raw == "" {
		raw = defaults.Url
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// ValidateRepository validates that the repository exists and is of the specified type
func ValidateRepository(repoKey string, rtDetails *config.ServerDetails, apiType string) error {
	log.Debug("Validating repository...")
	artDetails, err := rtDetails.CreateArtAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create auth config: %w", err)
	}

	if err := utils.ValidateRepoExists(repoKey, artDetails); err != nil {
		return fmt.Errorf("repository '%s' does not exist or is not accessible: %w", repoKey, err)
	}

	if err := utils.ValidateRepoType(repoKey, artDetails, apiType); err != nil {
		return fmt.Errorf("repository '%s' is not of type '%s': %w", repoKey, apiType, err)
	}

	log.Info("Repository validation successful")
	return nil
}

// GetBaseUrl extracts the base URL from server details
func GetBaseUrl(rtDetails *config.ServerDetails) string {
	baseUrl := rtDetails.ArtifactoryUrl
	if baseUrl == "" {
		baseUrl = rtDetails.Url
	}
	return strings.TrimRight(baseUrl, "/")
}

// ExtractRepoKeyFromURL extracts repository key from a URL containing the API type
func ExtractRepoKeyFromURL(urlStr, apiType string) string {
	parts := strings.Split(urlStr, "/")
	for i, p := range parts {
		if p == apiType && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// IsValidUrl checks if a string is a valid URL with scheme and host
func IsValidUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// BuildURL builds a full URL for a repository
func BuildURL(baseUrl, apiType, repoKey, urlSuffix string) string {
	if urlSuffix == "" {
		return fmt.Sprintf("%s/api/%s/%s", baseUrl, apiType, repoKey)
	}
	return fmt.Sprintf("%s/api/%s/%s/%s", baseUrl, apiType, repoKey, strings.TrimLeft(urlSuffix, "/"))
}

// NormalizeArtifactoryBaseUrl inspects a user-provided --url value and returns
// either a normalized Artifactory base URL or an error describing why the URL
// cannot be a base URL
func NormalizeArtifactoryBaseUrl(rawUrl, repoKey string) (string, error) {
	if rawUrl == "" {
		return rawUrl, nil
	}
	u, err := url.Parse(rawUrl)
	if err != nil || u.Host == "" {
		return rawUrl, nil
	}
	trimmedPath := strings.Trim(u.Path, "/")
	if trimmedPath == "" {
		return rawUrl, nil
	}
	segments := strings.Split(trimmedPath, "/")

	// Case A: URL contains an "/api/" segment.
	for _, seg := range segments {
		if strings.EqualFold(seg, "api") {
			return "", fmt.Errorf(
				"--url %q is a full API/service URL, not an Artifactory base URL. "+
					"Use the base URL (e.g. https://acme.jfrog.io/artifactory) and pass the "+
					"repository via --repo-key. Or, pass "+
					"the full service URL as the SERVICE_URL positional argument",
				rawUrl)
		}
	}

	artIndex := -1
	for i, seg := range segments {
		if strings.EqualFold(seg, "artifactory") {
			artIndex = i
			break
		}
	}

	// Case B: trailing segment matches --repo-key → strip it.
	last := segments[len(segments)-1]
	if repoKey != "" && strings.EqualFold(last, repoKey) {
		return rebuildUrlWithoutLastSegment(u, rawUrl), nil
	}

	// Case C: extra segments after "/artifactory/"
	if artIndex >= 0 && artIndex < len(segments)-1 {
		extra := strings.Join(segments[artIndex+1:], "/")
		hint := ""
		if repoKey != "" {
			hint = fmt.Sprintf(" (--repo-key is set to %q, which does not match)", repoKey)
		}
		return "", fmt.Errorf(
			"--url %q includes a path segment after '/artifactory/' (%q)%s. "+
				"--url must be the Artifactory base URL (e.g. https://acme.jfrog.io/artifactory), "+
				"without a repository key. Move the repository name to --repo-key",
			rawUrl, extra, hint)
	}

	return rawUrl, nil
}

// rebuildUrlWithoutLastSegment returns rawUrl with the last non-empty path
// segment removed, preserving the trailing slash if the original had one.
func rebuildUrlWithoutLastSegment(u *url.URL, rawUrl string) string {
	trailingSlash := strings.HasSuffix(rawUrl, "/")
	trimmed := strings.Trim(u.Path, "/")
	segs := strings.Split(trimmed, "/")
	segs = segs[:len(segs)-1]
	newPath := "/" + strings.Join(segs, "/")
	if newPath == "/" {
		newPath = ""
	} else if trailingSlash {
		newPath += "/"
	}
	nu := *u
	nu.Path = newPath
	return nu.String()
}
