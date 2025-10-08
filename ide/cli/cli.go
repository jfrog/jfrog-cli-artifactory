package cli

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/docs/setup"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const ideCategory = "IDE Integration"

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "setup",
			Description: setup.GetDescription(),
			Arguments:   setup.GetArguments(),
			Action:      setupCmd,
			Aliases:     []string{"s"},
			Category:    ideCategory,
		},
	}
}

func setupCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		errorMsg := "error: Missing mandatory argument 'IDE_NAME'. Please specify ide name. Supported IDEs are 'vscode' or 'jetbrains'"
		if c.PrintCommandHelp != nil {
			return pluginsCommon.PrintHelpAndReturnError(errorMsg, c)
		}
		return errors.New(errorMsg)
	}

	ideName := c.GetArgumentAt(0)
	if !isValidIDE(ideName) {
		errorMsg := fmt.Sprintf("error: Invalid IDE name '%s'. Supported IDEs are 'vscode' or 'jetbrains'", ideName)
		if c.PrintCommandHelp != nil {
			return pluginsCommon.PrintHelpAndReturnError(errorMsg, c)
		}
		return errors.New(errorMsg)
	}

	switch ideName {
	case "vscode":
		log.Info("VSCode setup selected.")
		return nil
	case "jetbrains":
		log.Info("JetBrains setup selected.")
		return nil
	}

	return nil
}

func isValidIDE(name string) bool {
	return name == "vscode" || name == "jetbrains"
}
