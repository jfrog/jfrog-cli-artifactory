package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCommands(t *testing.T) {
	commands := GetCommands()
	assert.NotEmpty(t, commands)
	assert.Equal(t, 1, len(commands), "Should have 1 IDE command (setup)")

	// Verify setup command
	assert.Equal(t, "setup", commands[0].Name)
	assert.Contains(t, commands[0].Aliases, "s")
	assert.Equal(t, ideCategory, commands[0].Category)
	assert.NotEmpty(t, commands[0].Arguments, "Setup command should have arguments")
}
