package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_HasPublishSubcommand(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 1)

	publish := commands[0]
	assert.Equal(t, "publish", publish.Name)
	assert.NotNil(t, publish.Action)
	require.Len(t, publish.Arguments, 1)
	assert.Equal(t, "path", publish.Arguments[0].Name)
	assert.Contains(t, publish.Description, "Publish an AI agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")
}
