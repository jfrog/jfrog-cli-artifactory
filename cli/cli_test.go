package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetJfrogCliArtifactoryApp_TopLevelSkillsNamespace(t *testing.T) {
	app := GetJfrogCliArtifactoryApp()

	skillsNS := findNamespaceByName(app.Subcommands, "skills")
	require.NotNil(t, skillsNS, "top-level 'skills' namespace should exist for backward compatibility")
	require.Equal(t, []string{"skill"}, skillsNS.Aliases)

	agentNS := findNamespaceByName(app.Subcommands, "agent")
	require.NotNil(t, agentNS)
	agentSkills := findCommandByName(agentNS.Commands, "skills")
	require.NotNil(t, agentSkills)

	want := []string{"list", "publish", "install", "update", "search", "delete"}
	topLevel := commandNames(skillsNS.Commands)
	agentLevel := commandNames(agentSkills.Subcommands)
	require.Equal(t, want, topLevel)
	require.Equal(t, want, agentLevel)
	for i := range want {
		require.NotNil(t, skillsNS.Commands[i].Action)
		require.NotNil(t, agentSkills.Subcommands[i].Action)
	}
}

func commandNames(commands []components.Command) []string {
	names := make([]string, 0, len(commands))
	for _, cmd := range commands {
		names = append(names, cmd.Name)
	}
	return names
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
