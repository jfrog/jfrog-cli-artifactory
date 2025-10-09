package commands

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	ideVSCode    = "vscode"
	ideJetBrains = "jetbrains"
)

// SetupCmd routes the setup command to the appropriate IDE handler
func SetupCmd(c *components.Context, ideName string) error {
	switch ideName {
	case ideVSCode:
		return SetupVscode(c)
	case ideJetBrains:
		return SetupJetbrains(c)
	default:
		return fmt.Errorf("unsupported IDE: %s", ideName)
	}
}

func SetupVscode(c *components.Context) error {
	return nil
}

func SetupJetbrains(c *components.Context) error {
	return nil
}
