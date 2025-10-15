package cli

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/commands"
	"github.com/jfrog/jfrog-cli-artifactory/ide/docs/setup"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const ideCategory = "IDE Integration"

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "setup",
			Description: setup.GetDescription(),
			Arguments:   setup.GetArguments(),
			Flags:       commands.GetSetupFlags(),
			Action:      setupCmd,
			Aliases:     []string{"s"},
			Category:    ideCategory,
		},
	}
}

func setupCmd(c *components.Context) error {
	numArgs := c.GetNumberOfArgs()
	if numArgs < 1 {
		errorMsg := fmt.Sprintf("error: Missing mandatory argument 'IDE_NAME'. Please specify ide name. Supported IDEs are %s", supportedIDEs())
		if c.PrintCommandHelp != nil {
			return common.WrongNumberOfArgumentsHandler(c)
		}
		return errors.New(errorMsg)
	}
	ideName := c.GetArgumentAt(0)
	if !isValidIDE(ideName) {
		errorMsg := fmt.Sprintf("error: Invalid IDE name '%s'. Supported IDEs are %s", ideName, supportedIDEs())
		if c.PrintCommandHelp != nil {
			return common.WrongNumberOfArgumentsHandler(c)
		}
		return errors.New(errorMsg)
	}
	return commands.SetupCmd(c, ideName)
}

func isValidIDE(name string) bool {
	switch name {
	case commands.IdeVSCode, commands.IdeJetBrains:
		return true
	default:
		return false
	}
}

func supportedIDEs() []string {
	return []string{commands.IdeVSCode, commands.IdeJetBrains}
}
