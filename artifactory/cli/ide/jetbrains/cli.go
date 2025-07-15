package jetbrains

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/jetbrains"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "jetbrains-config",
			Aliases:     []string{"jetbrains"},
			Action:      jetbrainsConfigCmd,
			Description: ide.JetbrainsConfigDescription,
		},
	}
}

func jetbrainsConfigCmd(c *components.Context) error {
	repositoryURL, err := ide.ValidateSingleNonEmptyArg(c, "jf jetbrains-config <repository-url>")
	if err != nil {
		return err
	}

	// Extract repo key from repository URL for potential validation
	repoKey := extractRepoKeyFromRepositoryURL(repositoryURL)

	// Create server details only if server configuration flags are provided
	// This makes server configuration optional for basic JetBrains setup
	var rtDetails *config.ServerDetails

	if ide.HasServerConfigFlags(c) {
		rtDetails, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			return fmt.Errorf("failed to create server configuration: %w", err)
		}
	}

	jetbrainsCmd := jetbrains.NewJetbrainsCommand(repositoryURL, repoKey)
	if rtDetails != nil {
		jetbrainsCmd.SetServerDetails(rtDetails)
	}

	return jetbrainsCmd.Run()
}

// extractRepoKeyFromRepositoryURL extracts the repository key from a JetBrains repository URL
// Expected format: https://<server>/artifactory/api/jetbrainsplugins/<repo-key>
func extractRepoKeyFromRepositoryURL(repositoryURL string) string {
	if repositoryURL == "" {
		return ""
	}

	// Parse the URL to extract the repository key
	parsedURL, err := url.Parse(repositoryURL)
	if err != nil {
		return ""
	}

	// Split the path to find the repository key
	// Expected path: /artifactory/api/jetbrainsplugins/<repo-key>
	pathParts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")

	// Look for the jetbrainsplugins API path
	for i, part := range pathParts {
		if part == "api" && i+1 < len(pathParts) && pathParts[i+1] == "jetbrainsplugins" && i+2 < len(pathParts) {
			return pathParts[i+2]
		}
	}

	return ""
}
