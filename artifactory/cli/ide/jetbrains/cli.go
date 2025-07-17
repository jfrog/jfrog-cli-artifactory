package jetbrains

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/jetbrains"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const (
	repoKeyFlag   = "repo-key"
	urlSuffixFlag = "url-suffix"
	apiType       = "jetbrainsplugins"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "jetbrains-config",
			Aliases:     []string{"jb"},
			Hidden:      true,
			Flags:       getFlags(),
			Arguments:   getArguments(),
			Action:      jetbrainsConfigCmd,
			Description: ide.JetbrainsConfigDescription,
		},
	}
}

func getFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(repoKeyFlag, "Repository key for the JetBrains plugins repo. [Required if no URL is given]", components.SetMandatoryFalse()),
		components.NewStringFlag(urlSuffixFlag, "Suffix for the JetBrains plugins repository URL. Default: (empty)", components.SetMandatoryFalse()),
		// Server configuration flags
		components.NewStringFlag("url", "JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}
}

func getArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "repository-url",
			Description: "The Artifactory JetBrains plugins repository URL (optional when using --repo-key)",
			Optional:    true,
		},
	}
}

// Main command action: orchestrates argument parsing, server config, and command execution
func jetbrainsConfigCmd(c *components.Context) error {
	repositoryURL, repoKey, err := getJetbrainsRepoKeyAndURL(c)
	if err != nil {
		return err
	}

	rtDetails, err := getJetbrainsServerDetails(c)
	if err != nil {
		return err
	}

	jetbrainsCmd := jetbrains.NewJetbrainsCommand(repositoryURL, repoKey)
	if rtDetails != nil {
		jetbrainsCmd.SetServerDetails(rtDetails)
	}

	return jetbrainsCmd.Run()
}

// getJetbrainsRepoKeyAndURL determines the repo key and repository URL from args/flags
func getJetbrainsRepoKeyAndURL(c *components.Context) (repoKey, repositoryURL string, err error) {
	if c.GetNumberOfArgs() > 0 && isValidUrl(c.GetArgumentAt(0)) {
		repositoryURL = c.GetArgumentAt(0)
		repoKey, err = extractRepoKeyFromRepositoryURL(repositoryURL)
		if err != nil {
			return
		}
		return
	}

	repoKey = c.GetStringFlagValue(repoKeyFlag)
	if repoKey == "" {
		err = fmt.Errorf("You must provide either a repository URL as the first argument or --repo-key flag.")
		return
	}
	// Get Artifactory URL from server details (flags or default)
	var artDetails *config.ServerDetails
	if ide.HasServerConfigFlags(c) {
		artDetails, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			err = fmt.Errorf("Failed to get Artifactory server details: %w", err)
			return
		}
	} else {
		artDetails, err = config.GetDefaultServerConf()
		if err != nil {
			err = fmt.Errorf("Failed to get default Artifactory server details: %w", err)
			return
		}
	}
	baseUrl := strings.TrimRight(artDetails.Url, "/")
	urlSuffix := c.GetStringFlagValue(urlSuffixFlag)
	if urlSuffix != "" {
		urlSuffix = "/" + strings.TrimLeft(urlSuffix, "/")
	}
	repositoryURL = baseUrl + "/artifactory/api/jetbrainsplugins/" + repoKey + urlSuffix
	return
}

// extractRepoKeyFromRepositoryURL extracts the repo key from a JetBrains plugins repository URL.
func extractRepoKeyFromRepositoryURL(repositoryURL string) (string, error) {
	if repositoryURL == "" {
		return "", fmt.Errorf("repository URL is empty")
	}
	trimmed := strings.TrimSuffix(repositoryURL, "/")
	parts := strings.Split(trimmed, "/api/jetbrainsplugins/")
	if len(parts) != 2 {
		return "", fmt.Errorf("repository URL does not contain /api/jetbrainsplugins/")
	}
	pathParts := strings.SplitN(parts[1], "/", 2)
	if len(pathParts) == 0 || pathParts[0] == "" {
		return "", fmt.Errorf("repository key not found in repository URL")
	}
	return pathParts[0], nil
}

// getJetbrainsServerDetails returns server details for validation, or nil if not available
func getJetbrainsServerDetails(c *components.Context) (*config.ServerDetails, error) {
	if ide.HasServerConfigFlags(c) {
		// Use explicit server configuration flags
		rtDetails, err := pluginsCommon.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			return nil, fmt.Errorf("failed to create server configuration: %w", err)
		}
		return rtDetails, nil
	}
	// Use default server configuration for validation when no explicit flags provided
	rtDetails, err := config.GetDefaultServerConf()
	if err != nil {
		// If no default server, that's okay - we'll just skip validation
		log.Debug("No default server configuration found, skipping repository validation")
		return nil, nil
	}
	return rtDetails, nil
}

func isValidUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
