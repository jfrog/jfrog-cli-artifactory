package jetbrains

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// SetupJetBrains configures JetBrains IDEs to use JFrog Artifactory
func SetupJetBrains(c *components.Context) error {
	// Parse common configuration using base logic
	baseConfig, err := ParseBaseSetupConfig(c)
	if err != nil {
		return err
	}

	// Create JetBrains command
	jetbrainsCmd := NewJetbrainsCommand(baseConfig.RepositoryURL, baseConfig.RepoKey)
	jetbrainsCmd.SetDirectURL(baseConfig.IsDirectURL)

	if baseConfig.ServerDetails != nil {
		jetbrainsCmd.SetServerDetails(baseConfig.ServerDetails)
	}

	return jetbrainsCmd.Run()
}
