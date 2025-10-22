package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/ide/commands/aieditorextensions"
	"github.com/jfrog/jfrog-cli-artifactory/ide/commands/jetbrains"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// GetCommands returns all IDE integration commands
func GetCommands() []components.Command {
	return []components.Command{
		GetSetupCommand(),
	}
}

// GetSetupCommand returns the setup command with IDE_NAME as argument
func GetSetupCommand() components.Command {
	return components.Command{
		Name:        "setup",
		Description: "Setup IDE integration with JFrog Artifactory.",
		Arguments:   getSetupArguments(),
		Flags:       getSetupFlags(),
		Action:      setupCmd,
		Aliases:     []string{"s"},
		Category:    ideCategory,
	}
}

func getSetupArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "ide-name",
			Description: "IDE to setup. Supported: vscode, cursor, windsurf, jetbrains",
		},
		{
			Name:        "url",
			Description: "[Optional] Direct repository/service URL. When provided, --repo-key and server config are not required.",
			Optional:    true,
		},
	}
}

func getSetupFlags() []components.Flag {
	// Start with common server flags
	flags := GetCommonServerFlags()

	// Add IDE-specific flags
	ideSpecificFlags := []components.Flag{
		// Repository flags
		components.NewStringFlag("repo-key", "Repository key. Required unless URL is provided as argument.", components.SetMandatoryFalse()),
		components.NewStringFlag("url-suffix", "Suffix for the URL. Optional.", components.SetMandatoryFalse()),

		// VSCode-specific flags
		components.NewStringFlag("product-json-path", "Path to VSCode/Cursor/Windsurf product.json file. If not provided, auto-detects installation.", components.SetMandatoryFalse()),
		components.NewStringFlag("update-mode", "VSCode update mode: 'default' (auto-update), 'manual' (prompt for updates), or 'none' (disable updates). Only for VSCode-based IDEs.", components.SetMandatoryFalse()),
	}

	return append(flags, ideSpecificFlags...)
}

func setupCmd(c *components.Context) error {
	if c.GetNumberOfArgs() == 0 {
		return errors.New("IDE_NAME is required. Usage: jf ide setup <IDE_NAME>\nSupported IDEs: vscode, cursor, windsurf, jetbrains")
	}

	ideName := strings.ToLower(c.GetArgumentAt(0))
	log.Debug(fmt.Sprintf("Setting up IDE: %s", ideName))

	switch ideName {
	case "vscode", "code":
		return aieditorextensions.SetupVSCode(c)
	case "cursor":
		return aieditorextensions.SetupCursor(c)
	case "windsurf":
		return aieditorextensions.SetupWindsurf(c)
	case "jetbrains", "jb":
		return jetbrains.SetupJetBrains(c)
	default:
		return fmt.Errorf("unsupported IDE: %s. Supported IDEs: vscode, cursor, windsurf, jetbrains", ideName)
	}
}
