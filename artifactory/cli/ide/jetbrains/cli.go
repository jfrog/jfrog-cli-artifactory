package jetbrains

import (
	"fmt"
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
	repoKey, repositoryURL, err := getJetbrainsRepoKeyAndURL(c)
	if err != nil {
		return err
	}

	rtDetails, err := getJetbrainsServerDetails(c)
	if err != nil {
		return err
	}

	jetbrainsCmd := jetbrains.NewJetbrainsCommand(repositoryURL, repoKey)

	// Determine if this is a direct URL (argument provided) vs constructed URL (server-id + repo-key)
	isDirectURL := c.GetNumberOfArgs() > 0 && ide.IsValidUrl(c.GetArgumentAt(0))
	jetbrainsCmd.SetDirectURL(isDirectURL)

	if rtDetails != nil {
		jetbrainsCmd.SetServerDetails(rtDetails)
	}

	return jetbrainsCmd.Run()
}

// getJetbrainsRepoKeyAndURL determines the repo key and repository URL from args/flags
func getJetbrainsRepoKeyAndURL(c *components.Context) (repoKey, repositoryURL string, err error) {
	if c.GetNumberOfArgs() > 0 && ide.IsValidUrl(c.GetArgumentAt(0)) {
		repositoryURL = c.GetArgumentAt(0)
		repoKey, err = ide.ExtractRepoKeyFromURL(repositoryURL)
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
	// Use ArtifactoryUrl if available (when using flags), otherwise use Url (when using config)
	baseUrl := artDetails.ArtifactoryUrl
	if baseUrl == "" {
		baseUrl = artDetails.Url
	}
	baseUrl = strings.TrimRight(baseUrl, "/")

	urlSuffix := c.GetStringFlagValue(urlSuffixFlag)
	if urlSuffix != "" {
		urlSuffix = "/" + strings.TrimLeft(urlSuffix, "/")
	}
	repositoryURL = baseUrl + "/api/jetbrainsplugins/" + repoKey + urlSuffix
	return
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
		return nil, nil //nolint:nilerr // Intentionally ignoring error to skip validation when no default server
	}
	return rtDetails, nil
}
