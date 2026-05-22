package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAiCommands_HasPluginsNamespace(t *testing.T) {
	commands := GetAiCommands()
	require.Len(t, commands, 1)

	plugins := commands[0]
	assert.Equal(t, "plugins", plugins.Name)
	assert.Nil(t, plugins.Action)
	require.Len(t, plugins.Subcommands, 1)
	assert.Equal(t, "publish", plugins.Subcommands[0].Name)
	assert.NotNil(t, plugins.Subcommands[0].Action)
}

func TestGetAiCommands_PluginsPublishDescription(t *testing.T) {
	commands := GetAiCommands()
	publish := commands[0].Subcommands[0]
	assert.Contains(t, publish.Description, "Publish an AI agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")
}
