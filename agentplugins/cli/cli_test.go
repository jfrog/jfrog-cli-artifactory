package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAiCommandsPluginsHasPublishSubcommand(t *testing.T) {
	commands := GetAiCommands()
	require.Len(t, commands, 1)
	assert.Equal(t, "plugins", commands[0].Name)
	assert.Nil(t, commands[0].Action)
	require.Len(t, commands[0].Subcommands, 1)
	assert.Equal(t, "publish", commands[0].Subcommands[0].Name)
	assert.NotNil(t, commands[0].Subcommands[0].Action)
	require.Len(t, commands[0].Subcommands[0].Arguments, 1)
	assert.Equal(t, "path", commands[0].Subcommands[0].Arguments[0].Name)
	assert.Contains(t, commands[0].Subcommands[0].Description, "{repo}/{plugin-slug}/{version}/")
	assert.Contains(t, commands[0].Subcommands[0].Description, "evidence attachment fails")
}
