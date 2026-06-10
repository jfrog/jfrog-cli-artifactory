package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_HasPublishInstallUpdateAndDelete(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 6)

	byName := make(map[string]components.Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	publish := byName["publish"]
	assert.NotNil(t, publish.Action)
	require.Len(t, publish.Arguments, 1)
	assert.Equal(t, "path", publish.Arguments[0].Name)
	assert.Equal(t, "Publish an agent plugin to Artifactory.", publish.Description)
	assert.Equal(t, "Path to the agent plugin folder containing plugin.json.", publish.Arguments[0].Description)

	installCmd := byName["install"]
	assert.NotNil(t, installCmd.Action)
	require.Len(t, installCmd.Arguments, 1)
	assert.Equal(t, "slug", installCmd.Arguments[0].Name)
	assert.Equal(t, "Install an agent plugin from Artifactory.", installCmd.Description)
	assert.Equal(t, "Agent plugin slug to install.", installCmd.Arguments[0].Description)

	updateCmd := byName["update"]
	assert.NotNil(t, updateCmd.Action)
	assert.Empty(t, updateCmd.Arguments)
	assert.Equal(t, "Update an installed agent plugin.", updateCmd.Description)

	del := byName["delete"]
	assert.NotNil(t, del.Action)
	require.Len(t, del.Arguments, 1)
	assert.Equal(t, "slug", del.Arguments[0].Name)
	assert.Equal(t, "Delete a specific agent plugin version from Artifactory.", del.Description)
	assert.Equal(t, "Agent plugin slug to delete.", del.Arguments[0].Description)

	lst := byName["list"]
	assert.NotNil(t, lst.Action)
	assert.Equal(t, "List agent plugins from Artifactory or on the local machine.", lst.Description)

	srch := byName["search"]
	assert.NotNil(t, srch.Action)
	require.Len(t, srch.Arguments, 1)
	assert.Equal(t, "query", srch.Arguments[0].Name)
	assert.Equal(t, "Search for agent plugins in Artifactory.", srch.Description)
	assert.Equal(t, "Agent plugin name or search term.", srch.Arguments[0].Description)
}
