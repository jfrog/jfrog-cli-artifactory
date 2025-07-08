package vscode

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/vscode"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	productJsonPath = "product-json-path"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:    "vscode-config",
			Aliases: []string{"vscode"},
			Flags:   getFlags(),
			Action:  vscodeConfigCmd,
			Description: `Configure VSCode to use JFrog Artifactory for extensions.

The service URL should be in the format:
https://<artifactory-url>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery

Examples:
  jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery
  jf rt vscode-config https://vscoded2c07.jfrogdev.org/artifactory/api/vscodeextensions/vscode-remote/_apis/public/gallery

This command will:
- Modify the VSCode product.json file to change the extensions gallery URL
- Create an automatic backup before making changes
- Require VSCode to be restarted to apply changes

Note: On macOS/Linux, you may need to run with sudo for system-installed VSCode.`,
		},
	}
}

func getFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag(productJsonPath, "[Optional] Path to VSCode product.json file. If not provided, auto-detects VSCode installation.", components.SetMandatoryFalse()),
	}
}

func vscodeConfigCmd(c *components.Context) error {
	if c.GetNumberOfArgs() == 0 {
		return fmt.Errorf("service URL is required\n\nUsage: jf rt vscode-config <service-url>\nExample: jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery")
	}

	serviceURL := c.GetArgumentAt(0)
	if serviceURL == "" {
		return fmt.Errorf("service URL cannot be empty\n\nUsage: jf rt vscode-config <service-url>")
	}

	productPath := c.GetStringFlagValue(productJsonPath)

	// Extract repo key from service URL for validation
	repoKey := extractRepoKeyFromServiceURL(serviceURL)

	// Create server details from flags
	rtDetails, err := pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	vscodeCmd := vscode.NewVscodeCommand(serviceURL, productPath, repoKey)
	vscodeCmd.SetServerDetails(rtDetails)

	return vscodeCmd.Run()
}

// extractRepoKeyFromServiceURL extracts the repository key from a VSCode service URL
// Expected format: https://<server>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery
func extractRepoKeyFromServiceURL(serviceURL string) string {
	if serviceURL == "" {
		return ""
	}

	// Look for the pattern: /api/vscodeextensions/<repo-key>/_apis/public/gallery
	const prefix = "/api/vscodeextensions/"
	const suffix = "/_apis/public/gallery"

	startIdx := strings.Index(serviceURL, prefix)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(prefix)

	endIdx := strings.Index(serviceURL[startIdx:], suffix)
	if endIdx == -1 {
		return ""
	}

	return serviceURL[startIdx : startIdx+endIdx]
}
