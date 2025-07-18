package vscode

import (
	"fmt"
	"strings"

	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/vscode"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const (
	productJsonPath = "product-json-path"
	repoKeyFlag     = "repo-key"
	urlSuffixFlag   = "url-suffix"
	apiType         = "vscodeextensions"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "vscode-config",
			Aliases:     []string{"vscode", "code"},
			Hidden:      true,
			Flags:       getFlags(),
			Arguments:   getArguments(),
			Action:      vscodeConfigCmd,
			Description: ide.VscodeConfigDescription,
		},
	}
}

func getFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(productJsonPath, "Path to VSCode product.json file. If not provided, auto-detects VSCode installation.", components.SetMandatoryFalse()),
		components.NewStringFlag(repoKeyFlag, "Repository key for the VSCode extensions repo. [Required if no URL is given]", components.SetMandatoryFalse()),
		components.NewStringFlag(urlSuffixFlag, "Suffix for the VSCode extensions service URL. Default: _apis/public/gallery", components.SetMandatoryFalse()),
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
			Name:        "service-url",
			Description: "The Artifactory VSCode extensions service URL (optional when using --repo-key)",
			Optional:    true,
		},
	}
}

// Main command action: orchestrates argument parsing, server config, and command execution
func vscodeConfigCmd(c *components.Context) error {
	repoKey, serviceURL, err := getVscodeRepoKeyAndURL(c)
	if err != nil {
		return err
	}

	productPath := c.GetStringFlagValue(productJsonPath)

	rtDetails, err := getVscodeServerDetails(c)
	if err != nil {
		return err
	}

	vscodeCmd := vscode.NewVscodeCommand(repoKey, productPath, serviceURL)

	// Determine if this is a direct URL (argument provided) vs constructed URL (server-id + repo-key)
	isDirectURL := c.GetNumberOfArgs() > 0 && ide.IsValidUrl(c.GetArgumentAt(0))

	vscodeCmd.SetDirectURL(isDirectURL)

	if rtDetails != nil {
		vscodeCmd.SetServerDetails(rtDetails)
	}

	return vscodeCmd.Run()
}

// getVscodeRepoKeyAndURL determines the repo key and service URL from args/flags
func getVscodeRepoKeyAndURL(c *components.Context) (repoKey, serviceURL string, err error) {
	if c.GetNumberOfArgs() > 0 && ide.IsValidUrl(c.GetArgumentAt(0)) {
		serviceURL = c.GetArgumentAt(0)
		repoKey, err = ide.ExtractRepoKeyFromURL(serviceURL)
		if err != nil {
			return
		}
		return
	}

	repoKey = c.GetStringFlagValue(repoKeyFlag)
	if repoKey == "" {
		err = fmt.Errorf("You must provide either a service URL as the first argument or --repo-key flag.")
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
	if urlSuffix == "" {
		urlSuffix = "_apis/public/gallery"
	}
	serviceURL = baseUrl + "/api/vscodeextensions/" + repoKey + "/" + strings.TrimLeft(urlSuffix, "/")
	return
}

// getVscodeServerDetails returns server details for validation, or nil if not available
func getVscodeServerDetails(c *components.Context) (*config.ServerDetails, error) {
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
