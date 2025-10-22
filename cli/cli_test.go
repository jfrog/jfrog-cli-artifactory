package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func TestGetJfrogCliArtifactoryApp(t *testing.T) {
	app := GetJfrogCliArtifactoryApp()

	// Verify rt namespace doesn't have IDE commands anymore
	rtNamespace := findNamespaceByName(app.Subcommands, "rt")
	assert.NotNil(t, rtNamespace, "rt namespace should exist")

	rtCommands := []string{"vscode-config", "jetbrains-config"}
	for _, cmdName := range rtCommands {
		cmd := findCommandByName(rtNamespace.Commands, cmdName)
		assert.Nil(t, cmd, "rt namespace should not contain %s command", cmdName)
	}
}

// Helper function to find a command by name
func findCommandByName(commands []components.Command, name string) *components.Command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}

// Helper function to find a namespace by name
func findNamespaceByName(namespaces []components.Namespace, name string) *components.Namespace {
	for i := range namespaces {
		if namespaces[i].Name == name {
			return &namespaces[i]
		}
	}
	return nil
}
