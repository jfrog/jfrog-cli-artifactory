package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectUnsupportedProjectScope_ClaudeRejected(t *testing.T) {
	err := RejectUnsupportedProjectScope(true, []AgentSpec{{Name: "claude"}}, "update")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude does not support project-scoped plugin updates")
	assert.Contains(t, err.Error(), "--global")
}

func TestRejectUnsupportedProjectScope_CursorRejected(t *testing.T) {
	err := RejectUnsupportedProjectScope(true, []AgentSpec{{Name: "Cursor"}}, "install")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor does not support project-scoped plugin installs")
}

func TestRejectUnsupportedProjectScope_CodexRejected(t *testing.T) {
	err := RejectUnsupportedProjectScope(true, []AgentSpec{{Name: "codex"}}, "update")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex does not support project-scoped plugin updates")
}

func TestRejectUnsupportedProjectScope_GlobalScopeAlwaysAllowed(t *testing.T) {
	err := RejectUnsupportedProjectScope(false, []AgentSpec{{Name: "claude"}, {Name: "cursor"}, {Name: "codex"}}, "update")
	require.NoError(t, err)
}

func TestRejectUnsupportedProjectScope_CustomAgentAllowed(t *testing.T) {
	err := RejectUnsupportedProjectScope(true, []AgentSpec{{Name: "my-custom-agent"}}, "update")
	require.NoError(t, err)
}

func TestRejectUnsupportedProjectScope_NoAgentsAllowed(t *testing.T) {
	err := RejectUnsupportedProjectScope(true, nil, "update")
	require.NoError(t, err)
}
