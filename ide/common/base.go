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

// GetServerDetails retrieves server configuration from flags or default config
func GetServerDetails(c *components.Context) (*config.ServerDetails, error) {
	if HasServerConfigFlags(c) {
		return pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	}
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

// BuildServerDetailsFromBaseURL constructs a ServerDetails from an explicit
// Artifactory base URL and the auth flags on ctx (--access-token, or
// --user + --password). Used when the caller has already resolved a base URL
// from a full API URL (e.g. --url with /api/<apiType>/<key>/... embedded) and
// therefore should not consult saved configs.
func BuildServerDetailsFromBaseURL(c *components.Context, baseURL string) (*config.ServerDetails, error) {
	baseURL = strings.TrimRight(baseURL, "/") + "/"
	details := &config.ServerDetails{
		ArtifactoryUrl: baseURL,
		Url:            baseURL,
	}
	if tok := c.GetStringFlagValue("access-token"); tok != "" {
		details.AccessToken = tok
		return details, nil
	}
	user := c.GetStringFlagValue("user")
	password := c.GetStringFlagValue("password")
	if user != "" && password != "" {
		details.User = user
		details.Password = password
		return details, nil
	}
	return nil, fmt.Errorf("credentials required: pass --access-token, or --user and --password")
}

// HasServerConfigFlags checks if any server configuration flags are provided
func HasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
}

// RequireAuthWhenUrlProvided returns an error when --url is supplied without
// enough information to authenticate. When --url is set, credentials must be
// provided explicitly via --access-token OR --user + --password.
//
// Note that --server-id is deliberately NOT accepted as a substitute here:
// upstream CreateArtifactoryDetailsByFlags (jfrog-cli-core) treats any --url
// as "explicit connection details" and skips loading credentials from the
// saved server-id, so combining --url with --server-id results in an
// anonymous request. To use a saved server, drop --url and pass --server-id
// alone (or leave both off to use the default server).
func RequireAuthWhenUrlProvided(c *components.Context) error {
	if !c.IsFlagSet("url") {
		return nil
	}
	if c.IsFlagSet("server-id") {
		return fmt.Errorf(
			"--url and --server-id cannot be combined; --server-id's saved credentials are ignored when --url is set. " +
				"Use --url with --access-token, or --url with --user and --password. " +
				"To use a saved server, drop --url and pass --server-id alone")
	}
	if c.IsFlagSet("access-token") {
		return nil
	}
	if c.IsFlagSet("user") && c.IsFlagSet("password") {
		return nil
	}
	return fmt.Errorf(
		"--url requires authentication details. " +
			"Pass --access-token, or --user and --password. " +
			"To use a server configured via 'jf config add', drop --url and pass --server-id alone")
}

// ValidateRepository validates that the repository exists and is of the specified type.
// Any error returned wraps the underlying transport / Artifactory response so the
// caller can see the real cause (e.g. 401 Unauthorized) instead of a generic
// "does not exist" message.
func ValidateRepository(repoKey string, rtDetails *config.ServerDetails, apiType string) error {
	log.Debug("Validating repository...")
	artDetails, err := rtDetails.CreateArtAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create auth config: %w", err)
	}

	if err := utils.ValidateRepoExists(repoKey, artDetails); err != nil {
		return fmt.Errorf("could not validate repository '%s': %w", repoKey, err)
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

// SplitApiURL takes a URL like https://host/artifactory/api/<apiType>/<repoKey>[/rest]
// and returns baseURL="https://host/artifactory", repoKey="<repoKey>", ok=true.
// Returns ok=false when the URL does not embed /api/<apiType>/<key>.
//
// Only the URL path is inspected; any query string or fragment is dropped.
// This prevents a URL like ".../<repo>?source=setup" from being interpreted as
// a repo key of "<repo>?source=setup".
func SplitApiURL(rawURL, apiType string) (baseURL, repoKey string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", false
	}
	marker := "/api/" + apiType + "/"
	idx := strings.Index(u.Path, marker)
	if idx < 0 {
		return "", "", false
	}
	basePath := strings.TrimRight(u.Path[:idx], "/")
	rest := u.Path[idx+len(marker):]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		repoKey = rest[:slash]
	} else {
		repoKey = rest
	}
	if repoKey == "" {
		return "", "", false
	}
	baseURL = u.Scheme + "://" + u.Host + basePath
	return baseURL, repoKey, true
}

// URLsHaveSameHost reports whether two URLs point at the same host.
// Used to guard against fetching a per-user token from one Artifactory
// instance and then writing it into a URL for a different one.
func URLsHaveSameHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil || ua.Host == "" || ub.Host == "" {
		return false
	}
	return strings.EqualFold(ua.Host, ub.Host)
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
