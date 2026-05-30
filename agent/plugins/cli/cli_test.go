package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_HasPublishAndInstall(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 2)

	publish := commands[0]
	assert.Equal(t, "publish", publish.Name)
	assert.NotNil(t, publish.Action)
	require.Len(t, publish.Arguments, 1)
	assert.Equal(t, "path", publish.Arguments[0].Name)
	assert.Contains(t, publish.Description, "Publish an agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")

	installCmd := commands[1]
	assert.Equal(t, "install", installCmd.Name)
	assert.NotNil(t, installCmd.Action)
	require.Len(t, installCmd.Arguments, 1)
	assert.Equal(t, "slug", installCmd.Arguments[0].Name)
	assert.Contains(t, installCmd.Description, "Install an agent plugin from Artifactory")
	assert.Contains(t, installCmd.Description, "marketplace")
}
