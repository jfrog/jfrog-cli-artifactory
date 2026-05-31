package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListCommand_NoMode(t *testing.T) {
	cmd := &ListCommand{}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jf agent plugins list requires")
}

func TestListCommand_BothModes(t *testing.T) {
	cmd := &ListCommand{repoKey: "my-repo", agentName: "claude"}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestListCommand_GlobalAndProjectDir(t *testing.T) {
	cmd := &ListCommand{agentName: "claude", global: true, projectDir: "/some/path"}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestListCommand_CheckUpdatesWithRepo(t *testing.T) {
	cmd := &ListCommand{repoKey: "my-repo", checkUpdates: true}
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--check-updates")
}
