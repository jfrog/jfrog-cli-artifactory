package aieditorextensions

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// SetupVSCode configures Visual Studio Code to use JFrog Artifactory
func SetupVSCode(c *components.Context) error {
	return setupVSCodeFork(c, "vscode")
}

// SetupCursor configures Cursor to use JFrog Artifactory
func SetupCursor(c *components.Context) error {
	return setupVSCodeFork(c, "cursor")
}

// SetupWindsurf configures Windsurf to use JFrog Artifactory
func SetupWindsurf(c *components.Context) error {
	return setupVSCodeFork(c, "windsurf")
}

// setupVSCodeFork is a generic setup function for any VSCode-based IDE
func setupVSCodeFork(c *components.Context, forkName string) error {
	// Get fork configuration
	forkConfig, exists := GetVSCodeFork(forkName)
	if !exists {
		return fmt.Errorf("unsupported IDE: %s", forkName)
	}

	// Parse common configuration using base logic
	baseConfig, err := ParseBaseSetupConfig(c)
	if err != nil {
		return err
	}

	// Get IDE-specific configuration
	productPath := c.GetStringFlagValue(ProductJsonPath)
	updateMode := c.GetStringFlagValue("update-mode")

	// Create generic VSCode fork command
	cmd := NewVSCodeForkCommand(forkConfig, baseConfig.RepoKey, productPath, baseConfig.ServiceURL)
	cmd.SetDirectURL(baseConfig.IsDirectURL)

	if baseConfig.ServerDetails != nil {
		cmd.SetServerDetails(baseConfig.ServerDetails)
	}

	// Set update mode if specified
	if updateMode != "" {
		cmd.SetUpdateMode(updateMode)
	}

	return cmd.Run()
}
