package cli

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/commands/aieditorextensions"
	"github.com/jfrog/jfrog-cli-artifactory/ide/commands/jetbrains"
	"github.com/jfrog/jfrog-cli-artifactory/ide/docs"
	"github.com/jfrog/jfrog-cli-artifactory/ide/ideconsts"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "setup",
			Description: docs.GetDescription(),
			Arguments:   docs.GetArguments(),
			Flags:       getSetupFlags(),
			Action:      setupCmd,
			Aliases:     []string{"s"},
			Category:    ideCategory,
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
		components.NewStringFlag("product-json-path", fmt.Sprintf("Path to %s product.json file. If not provided, auto-detects installation.", ideconsts.GetVSCodeBasedIDEsString()), components.SetMandatoryFalse()),
		components.NewStringFlag("update-mode", "Update mode: 'default' (auto-update), 'manual' (prompt for updates), or 'none' (disable updates). Only for VSCode-based IDEs.", components.SetMandatoryFalse()),
	}

	return append(flags, ideSpecificFlags...)
}

func setupCmd(ctx *components.Context) error {
	if ctx.GetNumberOfArgs() == 0 {
		return fmt.Errorf("IDE_NAME is required. Usage: jf ide setup <IDE_NAME>\nSupported IDEs: %s", ideconsts.GetSupportedIDEsString())
	}

	ideName := ctx.GetArgumentAt(0)
	log.Debug(fmt.Sprintf("Setting up IDE: %s", ideName))

	switch ideName {
	case ideconsts.IDENameVSCode, ideconsts.IDENameCode:
		return aieditorextensions.SetupVSCode(ctx)
	case ideconsts.IDENameCursor:
		return aieditorextensions.SetupCursor(ctx)
	case ideconsts.IDENameWindsurf:
		return aieditorextensions.SetupWindsurf(ctx)
	case ideconsts.IDENameKiro:
		return aieditorextensions.SetupKiro(ctx)
	case ideconsts.IDENameJetBrains, ideconsts.IDENameJB:
		return jetbrains.SetupJetBrains(ctx)
	default:
		return fmt.Errorf("unsupported IDE: %s. Supported IDEs: %s", ideName, ideconsts.GetSupportedIDEsString())
	}
}
