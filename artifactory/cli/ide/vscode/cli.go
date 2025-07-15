package vscode

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/vscode"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const (
	productJsonPath = "product-json-path"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "vscode-config",
			Aliases:     []string{"vscode"},
			Flags:       getFlags(),
			Action:      vscodeConfigCmd,
			Description: ide.VscodeConfigDescription,
		},
	}
}

func getFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(productJsonPath, "[Optional] Path to VSCode product.json file. If not provided, auto-detects VSCode installation.", components.SetMandatoryFalse()),
	}
}

func vscodeConfigCmd(c *components.Context) error {
	serviceURL, err := ide.ValidateSingleNonEmptyArg(c, "jf vscode-config <service-url>")
	if err != nil {
		return err
	}

	productPath := c.GetStringFlagValue(productJsonPath)

	// Extract repo key from service URL for potential validation
	repoKey := extractRepoKeyFromServiceURL(serviceURL)

	// Create server details only if server configuration flags are provided
	// This makes server configuration optional for basic VS Code setup
	var rtDetails *config.ServerDetails

	if ide.HasServerConfigFlags(c) {
		rtDetails, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			return fmt.Errorf("failed to create server configuration: %w", err)
		}
	}

	vscodeCmd := vscode.NewVscodeCommand(serviceURL, productPath, repoKey)
	if rtDetails != nil {
		vscodeCmd.SetServerDetails(rtDetails)
	}

	return vscodeCmd.Run()
}

// extractRepoKeyFromServiceURL extracts the repository key from a VSCode service URL
// Expected format: https://<server>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery
func extractRepoKeyFromServiceURL(serviceURL string) string {
	if serviceURL == "" {
		return ""
	}

	// Parse the URL to extract the repository key
	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		return ""
	}

	// Split the path to find the repository key
	// Expected path: /artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery
	pathParts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")

	// Look for the vscodeextensions API path
	for i, part := range pathParts {
		if part == "api" && i+1 < len(pathParts) && pathParts[i+1] == "vscodeextensions" && i+2 < len(pathParts) {
			return pathParts[i+2]
		}
	}

	return ""
}
