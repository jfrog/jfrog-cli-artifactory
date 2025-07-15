package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func TestGetTopLevelIDECommands(t *testing.T) {
	commands := getTopLevelIDECommands()

	// Should have exactly 2 commands (VSCode and JetBrains)
	assert.Equal(t, 2, len(commands), "Expected exactly 2 IDE commands")

	// Check VSCode command
	vscodeCmd := findCommandByName(commands, "vscode-config")
	assert.NotNil(t, vscodeCmd, "VSCode command should be found")
	assert.Contains(t, vscodeCmd.Aliases, "vscode", "VSCode command should have 'vscode' alias")
	assert.Contains(t, vscodeCmd.Aliases, "code", "VSCode command should have 'code' alias")

	// Check JetBrains command
	jetbrainsCmd := findCommandByName(commands, "jetbrains-config")
	assert.NotNil(t, jetbrainsCmd, "JetBrains command should be found")
	assert.Contains(t, jetbrainsCmd.Aliases, "jetbrains", "JetBrains command should have 'jetbrains' alias")
	assert.Contains(t, jetbrainsCmd.Aliases, "jb", "JetBrains command should have 'jb' alias")

	// Verify descriptions contain top-level usage examples
	assert.Contains(t, vscodeCmd.Description, "jf vscode-config", "VSCode description should show top-level usage")
	assert.Contains(t, vscodeCmd.Description, "jf code", "VSCode description should show 'code' alias usage")
	assert.Contains(t, jetbrainsCmd.Description, "jf jetbrains-config", "JetBrains description should show top-level usage")
	assert.Contains(t, jetbrainsCmd.Description, "jf jb", "JetBrains description should show 'jb' alias usage")
}

func TestGetJfrogCliArtifactoryApp(t *testing.T) {
	app := GetJfrogCliArtifactoryApp()

	// Verify app has top-level IDE commands
	ideCommands := []string{"vscode-config", "jetbrains-config"}
	for _, cmdName := range ideCommands {
		cmd := findCommandByName(app.Commands, cmdName)
		assert.NotNil(t, cmd, "Top-level command %s should be found", cmdName)
	}

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
