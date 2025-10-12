package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	ideVSCode       = "vscode"
	ideJetBrains    = "jetbrains"
	repoKeyFlag     = "repo-key"
	urlSuffixFlag   = "url-suffix"
	productJsonPath = "product-json-path"
	apiType         = "aieditorextensions"
)

// SetupCmd routes the setup command to the appropriate IDE handler
func SetupCmd(c *components.Context, ideName string) error {
	switch ideName {
	case ideVSCode:
		return SetupVscode(c)
	case ideJetBrains:
		return SetupJetbrains(c)
	default:
		return fmt.Errorf("unsupported IDE: %s", ideName)
	}
}

func SetupVscode(c *components.Context) error {
	log.Info("Setting up VSCode IDE integration...")

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
	serviceURL := fmt.Sprintf("%s/api/%s/%s/%s", baseUrl, apiType, repoKey, strings.TrimLeft(urlSuffix, "/"))

	vscodeCmd := NewVscodeCommand(repoKey, productPath, serviceURL)
	vscodeCmd.SetServerDetails(rtDetails)
	vscodeCmd.SetDirectURL(false)
	return vscodeCmd.Run()
}

func SetupJetbrains(c *components.Context) error {
	log.Info("Setting up JetBrains IDEs integration...")

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
	repositoryURL := fmt.Sprintf("%s/api/%s/%s%s", baseUrl, apiType, repoKey, urlSuffix)

	jetbrainsCmd := NewJetbrainsCommand(repositoryURL, repoKey)
	jetbrainsCmd.SetServerDetails(rtDetails)
	jetbrainsCmd.SetDirectURL(false)

	return jetbrainsCmd.Run()
}

// GetJetbrainsSetupFlags returns the flags for the jetbrains setup command
func GetJetbrainsSetupFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(repoKeyFlag, "Repository key for the JetBrains plugins repository. [Required]", components.SetMandatoryFalse()),
		components.NewStringFlag(urlSuffixFlag, "Suffix for the JetBrains plugins repository URL. Default: (empty)", components.SetMandatoryFalse()),
		// Server configuration flags
		components.NewStringFlag("url", "JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}
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

// validateRepository validates that the repository exists and is type 'aieditorextensions'
func validateRepository(repoKey string, rtDetails *config.ServerDetails) error {
	log.Debug("Validating repository...")

	artDetails, err := rtDetails.CreateArtAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create auth config: %w", err)
	}

	if err := utils.ValidateRepoExists(repoKey, artDetails); err != nil {
		return fmt.Errorf("repository '%s' does not exist or is not accessible: %w", repoKey, err)
	}

	if err := utils.ValidateRepoType(repoKey, artDetails, apiType); err != nil {
		return fmt.Errorf("error: repository '%s' is not of type '%s'. Using other repo types is not supported. Please ensure you're using an AI Editor Extensions repository", repoKey, apiType)
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

// GetVscodeSetupFlags returns the flags for the vscode setup command
func GetVscodeSetupFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(repoKeyFlag, "Repository key for the VSCode extensions repository. [Required]", components.SetMandatoryFalse()),
		components.NewStringFlag(productJsonPath, "Path to VSCode product.json file. If not provided, auto-detects VSCode installation.", components.SetMandatoryFalse()),
		components.NewStringFlag(urlSuffixFlag, "Suffix for the VSCode extensions service URL. Default: _apis/public/gallery", components.SetMandatoryFalse()),
		// Server configuration flags
		components.NewStringFlag("url", "JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}
}

// GetSetupFlags returns the combined flags for all ide setup commands
// Deduplicates common flags between ide's
func GetSetupFlags() []components.Flag {
	vscodeFlags := GetVscodeSetupFlags()
	jetbrainsFlags := GetJetbrainsSetupFlags()

	// Use a map to deduplicate common flags (server config flags are shared)
	flagMap := make(map[string]components.Flag)
	for _, flag := range append(vscodeFlags, jetbrainsFlags...) {
		flagMap[flag.GetName()] = flag
	}

	// Convert map back to slice
	uniqueFlags := make([]components.Flag, 0, len(flagMap))
	for _, flag := range flagMap {
		uniqueFlags = append(uniqueFlags, flag)
	}

	return uniqueFlags
}
