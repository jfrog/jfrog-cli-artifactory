package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func TestGetSubCommands_UpdateSupportsAll(t *testing.T) {
	commands := GetSubCommands()
	byName := make(map[string]components.Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	updateCmd := byName["update"]
	assert.NotNil(t, updateCmd.Action)
	assert.Empty(t, updateCmd.Arguments)
	assert.Contains(t, updateCmd.Description, "Update an installed skill")
	assert.Contains(t, updateCmd.Description, "--slug")
	assert.Contains(t, updateCmd.Description, "--all")
}
