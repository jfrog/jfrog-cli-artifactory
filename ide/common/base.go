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
	if rtDetails.ArtifactoryUrl == "" && rtDetails.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}
	return rtDetails, nil
}

// HasServerConfigFlags checks if any server configuration flags are provided
func HasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
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
