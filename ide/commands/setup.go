package commands

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	IdeVSCode       = "vscode"
	IdeJetBrains    = "jetbrains"
	repoKeyFlag     = "repo-key"
	urlSuffixFlag   = "url-suffix"
	productJsonPath = "product-json-path"
	ApiType         = "aieditorextensions"
)

// SetupCmd routes the setup command to the appropriate IDE handler
func SetupCmd(c *components.Context, ideName string) error {
	switch ideName {
	case IdeVSCode:
		return SetupVscode(c)
	case IdeJetBrains:
		return SetupJetbrains(c)
	default:
		errorMsg := fmt.Sprintf("unsupported IDE: %s", ideName)
		return pluginsCommon.PrintHelpAndReturnError(errorMsg, c)
	}
}

func SetupVscode(c *components.Context) error {
	log.Info("Setting up VSCode IDE integration...")
	// If a direct repository URL is provided as the 2nd positional arg, use it as-is
	if c.GetNumberOfArgs() > 1 && IsValidUrl(c.GetArgumentAt(1)) {
		directURL := c.GetArgumentAt(1)
		repoKey := ""
		parts := strings.Split(directURL, "/")
		for i, p := range parts {
			if p == ApiType && i+1 < len(parts) {
				repoKey = parts[i+1]
				break
			}
		}
		productPath := c.GetStringFlagValue(productJsonPath)
		vscodeCmd := NewVscodeCommand(repoKey, productPath, directURL)
		vscodeCmd.SetDirectURL(true)
		return vscodeCmd.Run()
	}

	repoKey := c.GetStringFlagValue(repoKeyFlag)
	productPath := c.GetStringFlagValue(productJsonPath)
	urlSuffix := c.GetStringFlagValue(urlSuffixFlag)
	if urlSuffix == "" {
		urlSuffix = "_apis/public/gallery"
	}

	if repoKey == "" {
		return errors.New("--repo-key flag is required. Please specify the repository key for your VSCode extensions repository")
	}
	rtDetails, err := getServerDetails(c)
	if err != nil {
		return fmt.Errorf("failed to get server configuration: %w. Please run 'jf config add' first", err)
	}
	if err := validateRepository(repoKey, rtDetails); err != nil {
		return err
	}

	baseUrl := getBaseUrl(rtDetails)
	serviceURL := fmt.Sprintf("%s/api/%s/%s/%s", baseUrl, ApiType, repoKey, strings.TrimLeft(urlSuffix, "/"))
	vscodeCmd := NewVscodeCommand(repoKey, productPath, serviceURL)
	vscodeCmd.SetServerDetails(rtDetails)
	vscodeCmd.SetDirectURL(false)
	return vscodeCmd.Run()
}

func SetupJetbrains(c *components.Context) error {
	log.Info("Setting up JetBrains IDEs integration...")
	// If a direct repository URL is provided as the 2nd positional arg, use it as-is
	if c.GetNumberOfArgs() > 1 && IsValidUrl(c.GetArgumentAt(1)) {
		directURL := c.GetArgumentAt(1)
		repoKey := ""
		parts := strings.Split(directURL, "/")
		for i, p := range parts {
			if p == ApiType && i+1 < len(parts) {
				repoKey = parts[i+1]
				break
			}
		}
		jetbrainsCmd := NewJetbrainsCommand(directURL, repoKey)
		jetbrainsCmd.SetDirectURL(true)
		return jetbrainsCmd.Run()
	}

	repoKey := c.GetStringFlagValue(repoKeyFlag)
	urlSuffix := c.GetStringFlagValue(urlSuffixFlag)

	if repoKey == "" {
		return errors.New("--repo-key flag is required. Please specify the repository key for your JetBrains plugins repository")
	}
	rtDetails, err := getServerDetails(c)
	if err != nil {
		return fmt.Errorf("failed to get server configuration: %w. Please run 'jf config add' first", err)
	}
	if err := validateRepository(repoKey, rtDetails); err != nil {
		return err
	}
	baseUrl := getBaseUrl(rtDetails)
	if urlSuffix != "" {
		urlSuffix = "/" + strings.TrimLeft(urlSuffix, "/")
	}
	repositoryURL := fmt.Sprintf("%s/api/%s/%s%s", baseUrl, ApiType, repoKey, urlSuffix)
	jetbrainsCmd := NewJetbrainsCommand(repositoryURL, repoKey)
	jetbrainsCmd.SetServerDetails(rtDetails)
	jetbrainsCmd.SetDirectURL(false)
	return jetbrainsCmd.Run()
}

// getServerDetails retrieves server configuration from flags or default config
func getServerDetails(c *components.Context) (*config.ServerDetails, error) {
	if hasServerConfigFlags(c) {
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

// hasServerConfigFlags checks if any server configuration flags are provided
func hasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
}

// validateRepository validates that the repository exists and is type 'aieditorextension'
func validateRepository(repoKey string, rtDetails *config.ServerDetails) error {
	log.Debug("Validating repository...")
	artDetails, err := rtDetails.CreateArtAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create auth config: %w", err)
	}
	if err := utils.ValidateRepoExists(repoKey, artDetails); err != nil {
		return fmt.Errorf("repository '%s' does not exist or is not accessible: %w", repoKey, err)
	}
	if err := utils.ValidateRepoType(repoKey, artDetails, ApiType); err != nil {
		return fmt.Errorf("error: repository '%s' is not of type '%s'. Using other repo types is not supported. Please ensure you're using an AI Editor Extensions repository", repoKey, ApiType)
	}
	log.Info("Repository validation successful")
	return nil
}

// getBaseUrl extracts the base URL from server details
func getBaseUrl(rtDetails *config.ServerDetails) string {
	baseUrl := rtDetails.ArtifactoryUrl
	if baseUrl == "" {
		baseUrl = rtDetails.Url
	}
	return strings.TrimRight(baseUrl, "/")
}

// GetSetupFlags returns the combined flags for all ide setup commands
func GetSetupFlags() []components.Flag {
	// common server flags
	commonServerFlags := []components.Flag{
		components.NewStringFlag("url", "JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}

	// IDE-specific flags
	ideSpecificFlags := []components.Flag{
		components.NewStringFlag(repoKeyFlag, "Repository key for the AI Editor Extensions repository. [Required]", components.SetMandatoryFalse()),
		components.NewStringFlag(productJsonPath, "Path to VSCode product.json file. If not provided, auto-detects VSCode installation. (VSCode only)", components.SetMandatoryFalse()),
		components.NewStringFlag(urlSuffixFlag, "Suffix for the repository URL. Default: _apis/public/gallery for VSCode, empty for JetBrains", components.SetMandatoryFalse()),
	}

	return append(commonServerFlags, ideSpecificFlags...)
}

// IsValidUrl checks if a string is a valid URL with scheme and host
func IsValidUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
