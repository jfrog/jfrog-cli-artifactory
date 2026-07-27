package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommands_HasPluginsAndSkillsNamespaces(t *testing.T) {
	commands := GetCommands()
	require.Len(t, commands, 3)

	plugins := commands[0]
	assert.Equal(t, "plugins", plugins.Name)
	assert.Nil(t, plugins.Action)
	pluginsNames := make([]string, 0, len(plugins.Subcommands))
	for _, sub := range plugins.Subcommands {
		assert.NotNil(t, sub.Action, "plugins subcommand %q must have an Action", sub.Name)
		pluginsNames = append(pluginsNames, sub.Name)
	}
	assert.ElementsMatch(t, []string{"publish", "install", "update", "delete", "list", "search"}, pluginsNames)

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

	apm := commands[2]
	assert.Equal(t, "apm", apm.Name)
	// Unlike plugins/skills, apm's parent command has its own Action (RunApmPassthroughDefault)
	// so unregistered apm subcommands (doctor, list, ...) still reach the real apm binary.
	assert.NotNil(t, apm.Action)
	apmNames := make([]string, 0, len(apm.Subcommands))
	for _, sub := range apm.Subcommands {
		assert.NotNil(t, sub.Action, "apm subcommand %q must have an Action", sub.Name)
		apmNames = append(apmNames, sub.Name)
	}
	assert.ElementsMatch(t, []string{"install", "publish", "update"}, apmNames)
}

func TestGetCommands_PluginsPublishDescription(t *testing.T) {
	commands := GetCommands()
	publish := commands[0].Subcommands[0]
	assert.Equal(t, "publish", publish.Name)
	assert.Equal(t, "Publish an agent plugin to Artifactory.", publish.Description)
}
