package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_HasPublishSubcommand(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 2)

	publish := commands[0]
	assert.Equal(t, "publish", publish.Name)
	assert.NotNil(t, publish.Action)
	require.Len(t, publish.Arguments, 1)
	assert.Equal(t, "path", publish.Arguments[0].Name)
	assert.Contains(t, publish.Description, "Publish an agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")
}

func TestGetSubCommands_HasInstallSubcommand(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 2)

	install := commands[1]
	assert.Equal(t, "install", install.Name)
	assert.NotNil(t, install.Action)
	require.Len(t, install.Arguments, 1)
	assert.Equal(t, "slug", install.Arguments[0].Name)
	assert.Contains(t, install.Description, "Install an agent plugin from Artifactory")
	assert.NotEmpty(t, install.Flags)
}
