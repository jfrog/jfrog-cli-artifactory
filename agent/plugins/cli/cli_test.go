package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_HasPublishInstallUpdateAndDelete(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 4)

	byName := make(map[string]components.Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	publish := byName["publish"]
	assert.NotNil(t, publish.Action)
	require.Len(t, publish.Arguments, 1)
	assert.Equal(t, "path", publish.Arguments[0].Name)
	assert.Contains(t, publish.Description, "Publish an agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")

	installCmd := byName["install"]
	assert.NotNil(t, installCmd.Action)
	require.Len(t, installCmd.Arguments, 1)
	assert.Equal(t, "slug", installCmd.Arguments[0].Name)
	assert.Contains(t, installCmd.Description, "Install an agent plugin from Artifactory")
	assert.Contains(t, installCmd.Description, "marketplace")

	updateCmd := byName["update"]
	assert.NotNil(t, updateCmd.Action)
	assert.Empty(t, updateCmd.Arguments)
	assert.Contains(t, updateCmd.Description, "Update an installed plugin")
	assert.Contains(t, updateCmd.Description, "--slug")
	assert.Contains(t, updateCmd.Description, "--all")

	del := byName["delete"]
	assert.NotNil(t, del.Action)
	require.Len(t, del.Arguments, 1)
	assert.Equal(t, "slug", del.Arguments[0].Name)
	assert.Contains(t, del.Description, "Delete a specific agent plugin version")
}
