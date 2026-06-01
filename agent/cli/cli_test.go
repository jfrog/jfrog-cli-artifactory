package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommands_HasPluginsAndSkillsNamespaces(t *testing.T) {
	commands := GetCommands()
	require.Len(t, commands, 2)

	plugins := commands[0]
	assert.Equal(t, "plugins", plugins.Name)
	assert.Nil(t, plugins.Action)
	pluginsNames := make([]string, 0, len(plugins.Subcommands))
	for _, sub := range plugins.Subcommands {
		assert.NotNil(t, sub.Action, "plugins subcommand %q must have an Action", sub.Name)
		pluginsNames = append(pluginsNames, sub.Name)
	}
	assert.ElementsMatch(t, []string{"publish", "install", "delete", "list"}, pluginsNames)

	skills := commands[1]
	assert.Equal(t, "skills", skills.Name)
	assert.Nil(t, skills.Action)
	skillsNames := make([]string, 0, len(skills.Subcommands))
	for _, sub := range skills.Subcommands {
		assert.NotNil(t, sub.Action, "skills subcommand %q must have an Action", sub.Name)
		skillsNames = append(skillsNames, sub.Name)
	}
	assert.ElementsMatch(t,
		[]string{"list", "publish", "install", "update", "search", "delete"},
		skillsNames,
	)
}

func TestGetCommands_PluginsPublishDescription(t *testing.T) {
	commands := GetCommands()
	publish := commands[0].Subcommands[0]
	assert.Contains(t, publish.Description, "Publish an agent plugin to Artifactory")
	assert.Contains(t, publish.Description, "Signs and attaches evidence")
}
