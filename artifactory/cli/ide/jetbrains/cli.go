package jetbrains

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide/jetbrains"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:    "jetbrains-config",
			Aliases: []string{"jetbrains"},
			Action:  jetbrainsConfigCmd,
			Description: `Configure JetBrains IDEs to use JFrog Artifactory for plugins.

The repository URL should be in the format:
https://<artifactory-url>/artifactory/api/jetbrainsplugins/<repo-key>

Examples:
  jf rt jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins
  jf rt jetbrains-config https://vsjetd2c04.jfrogdev.org/artifactory/api/jetbrainsplugins/jetbrains-remote

This command will:
- Detect all installed JetBrains IDEs
- Modify each IDE's idea.properties file to add the plugins repository URL
- Create automatic backups before making changes
- Require IDEs to be restarted to apply changes

Supported IDEs: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua`,
		},
	}
}

func jetbrainsConfigCmd(c *components.Context) error {
	if c.GetNumberOfArgs() == 0 {
		return fmt.Errorf("repository URL is required\n\nUsage: jf rt jetbrains-config <repository-url>\nExample: jf rt jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins")
	}

	repositoryURL := c.GetArgumentAt(0)
	if repositoryURL == "" {
		return fmt.Errorf("repository URL cannot be empty\n\nUsage: jf rt jetbrains-config <repository-url>")
	}

	// Extract repo key from repository URL for validation
	repoKey := extractRepoKeyFromRepositoryURL(repositoryURL)

	// Create server details from flags
	rtDetails, err := pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	jetbrainsCmd := jetbrains.NewJetbrainsCommand(repositoryURL, repoKey)
	jetbrainsCmd.SetServerDetails(rtDetails)

	return jetbrainsCmd.Run()
}

// extractRepoKeyFromRepositoryURL extracts the repository key from a JetBrains repository URL
// Expected format: https://<server>/artifactory/api/jetbrainsplugins/<repo-key>
func extractRepoKeyFromRepositoryURL(repositoryURL string) string {
	if repositoryURL == "" {
		return ""
	}

	// Look for the pattern: /api/jetbrainsplugins/<repo-key>
	const prefix = "/api/jetbrainsplugins/"

	startIdx := strings.Index(repositoryURL, prefix)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(prefix)

	// Get everything after the prefix (until end of string or next slash)
	remaining := repositoryURL[startIdx:]
	endIdx := strings.Index(remaining, "/")
	if endIdx == -1 {
		// No trailing slash, use the rest of the string
		return remaining
	}

	return remaining[:endIdx]
}
