package vscode

import (
	"fmt"
	"net/url"
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

func vscodeConfigCmd(c *components.Context) error {
	var serviceURL, repoKey string
	var err error

	if c.GetNumberOfArgs() > 0 && isValidUrl(c.GetArgumentAt(0)) {
		serviceURL = c.GetArgumentAt(0)
		repoKey = extractRepoKeyFromServiceURL(serviceURL)
	} else {
		repoKey = c.GetStringFlagValue(repoKeyFlag)
		if repoKey == "" {
			return fmt.Errorf("You must provide either a service URL as the first argument or --repo-key flag.")
		}
		// Get Artifactory URL from server details (flags or default)
		var artDetails *config.ServerDetails
		if ide.HasServerConfigFlags(c) {
			artDetails, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
			if err != nil {
				return fmt.Errorf("Failed to get Artifactory server details: %w", err)
			}
		} else {
			artDetails, err = config.GetDefaultServerConf()
			if err != nil {
				return fmt.Errorf("Failed to get default Artifactory server details: %w", err)
			}
		}
		baseUrl := strings.TrimRight(artDetails.Url, "/")
		urlSuffix := c.GetStringFlagValue(urlSuffixFlag)
		if urlSuffix == "" {
			urlSuffix = "_apis/public/gallery"
		}
		serviceURL = baseUrl + "/artifactory/api/vscodeextensions/" + repoKey + "/" + strings.TrimLeft(urlSuffix, "/")
	}

	productPath := c.GetStringFlagValue(productJsonPath)

	// Create server details for validation
	var rtDetails *config.ServerDetails
	if ide.HasServerConfigFlags(c) {
		// Use explicit server configuration flags
		rtDetails, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			return fmt.Errorf("failed to create server configuration: %w", err)
		}
	} else {
		// Use default server configuration for validation when no explicit flags provided
		rtDetails, err = config.GetDefaultServerConf()
		if err != nil {
			// If no default server, that's okay - we'll just skip validation
			log.Debug("No default server configuration found, skipping repository validation")
		}
	}

	vscodeCmd := vscode.NewVscodeCommand(serviceURL, productPath, repoKey)
	if rtDetails != nil {
		vscodeCmd.SetServerDetails(rtDetails)
	}

	return vscodeCmd.Run()
}

func isValidUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// extractRepoKeyFromServiceURL extracts the repository key from a VSCode service URL
// Expected format: https://<server>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery
func extractRepoKeyFromServiceURL(serviceURL string) string {
	if serviceURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		return ""
	}

	pathParts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")
	for i, part := range pathParts {
		if part == "api" && i+1 < len(pathParts) && pathParts[i+1] == "vscodeextensions" && i+2 < len(pathParts) {
			return pathParts[i+2]
		}
	}
	return ""
}
