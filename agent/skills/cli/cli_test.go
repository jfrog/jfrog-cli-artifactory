package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubCommands_DescriptionsAndArguments(t *testing.T) {
	commands := GetSubCommands()
	require.Len(t, commands, 6)

	byName := make(map[string]components.Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	assert.Equal(t, "List skills from Artifactory or on the local machine.", byName["list"].Description)

	publish := byName["publish"]
	assert.Equal(t, "Publish a skill to Artifactory.", publish.Description)
	assert.Equal(t, "Path to the skill folder containing SKILL.md.", publish.Arguments[0].Description)

	installCmd := byName["install"]
	assert.Equal(t, "Install a skill from Artifactory.", installCmd.Description)
	assert.Equal(t, "Skill slug to install.", installCmd.Arguments[0].Description)

	updateCmd := byName["update"]
	assert.NotNil(t, updateCmd.Action)
	assert.Empty(t, updateCmd.Arguments)
	assert.Equal(t, "Update an installed skill.", updateCmd.Description)

	searchCmd := byName["search"]
	assert.Equal(t, "Search for skills in Artifactory.", searchCmd.Description)
	assert.Equal(t, "Skill name or search term.", searchCmd.Arguments[0].Description)

	del := byName["delete"]
	assert.Equal(t, "Delete a specific skill version from Artifactory.", del.Description)
	assert.Equal(t, "Skill slug to delete.", del.Arguments[0].Description)
}
