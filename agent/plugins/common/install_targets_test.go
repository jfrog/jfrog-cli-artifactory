package common

import (
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/stretchr/testify/assert"
)

func TestInjectRepoKey_Claude(t *testing.T) {
	targets := []AgentTarget{
		{
			Agent:          AgentSpec{Name: "claude"},
			DestinationDir: filepath.Join("/home/user/.claude/plugins/local", "my-plugin"),
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Len(t, result, 1)
	assert.Equal(t, filepath.Join("/home/user/.claude/plugins/local/my-repo", "my-plugin"), result[0].DestinationDir)
}

func TestInjectRepoKey_Codex(t *testing.T) {
	targets := []AgentTarget{
		{
			Agent:          AgentSpec{Name: "codex"},
			DestinationDir: filepath.Join("/home/user/.agents/plugins/local", "my-plugin"),
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Len(t, result, 1)
	assert.Equal(t, filepath.Join("/home/user/.agents/plugins/local/my-repo", "my-plugin"), result[0].DestinationDir)
}

func TestInjectRepoKey_Cursor_Unchanged(t *testing.T) {
	originalPath := filepath.Join("/home/user/.cursor/plugins/local", "my-plugin")
	targets := []AgentTarget{
		{
			Agent:          AgentSpec{Name: "cursor"},
			DestinationDir: originalPath,
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Len(t, result, 1)
	assert.Equal(t, originalPath, result[0].DestinationDir, "Cursor targets should not be modified")
}

func TestInjectRepoKey_UnknownAgent_Unchanged(t *testing.T) {
	originalPath := filepath.FromSlash("/custom/install/path/my-plugin")
	targets := []AgentTarget{
		{
			Agent:          AgentSpec{Name: "unknown-agent"},
			DestinationDir: originalPath,
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Len(t, result, 1)
	assert.Equal(t, originalPath, result[0].DestinationDir, "Unknown agents should not be modified")
}

func TestInjectRepoKey_CaseInsensitive(t *testing.T) {
	targets := []AgentTarget{
		{
			Agent:          AgentSpec{Name: "Claude"},
			DestinationDir: filepath.Join("/home/user/.claude/plugins/local", "my-plugin"),
		},
		{
			Agent:          AgentSpec{Name: "CODEX"},
			DestinationDir: filepath.Join("/home/user/.agents/plugins/local", "my-plugin"),
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Len(t, result, 2)
	assert.Equal(t, filepath.Join("/home/user/.claude/plugins/local/my-repo", "my-plugin"), result[0].DestinationDir)
	assert.Equal(t, filepath.Join("/home/user/.agents/plugins/local/my-repo", "my-plugin"), result[1].DestinationDir)
}

func TestInjectRepoKey_MultipleTargets(t *testing.T) {
	tests := []struct {
		name           string
		agentName      string
		inputPath      string
		expectedChange bool
	}{
		{
			name:           "claude_modified",
			agentName:      "claude",
			inputPath:      filepath.FromSlash("/home/user/.claude/plugins/local/plugin1"),
			expectedChange: true,
		},
		{
			name:           "codex_modified",
			agentName:      "codex",
			inputPath:      filepath.FromSlash("/home/user/.agents/plugins/local/plugin2"),
			expectedChange: true,
		},
		{
			name:           "cursor_unchanged",
			agentName:      "cursor",
			inputPath:      filepath.FromSlash("/home/user/.cursor/plugins/local/plugin3"),
			expectedChange: false,
		},
		{
			name:           "unknown_agent_unchanged",
			agentName:      "my-agent",
			inputPath:      filepath.FromSlash("/custom/path/plugin4"),
			expectedChange: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targets := []AgentTarget{
				{
					Agent:          AgentSpec{Name: tc.agentName},
					DestinationDir: tc.inputPath,
				},
			}

			result := InjectRepoKey(targets, "test-repo")

			if tc.expectedChange {
				expectedDir := filepath.Join(filepath.Dir(tc.inputPath), "test-repo", filepath.Base(tc.inputPath))
				assert.Equal(t, expectedDir, result[0].DestinationDir)
			} else {
				assert.Equal(t, tc.inputPath, result[0].DestinationDir)
			}
		})
	}
}

func TestInjectRepoKey_PreservesOtherFields(t *testing.T) {
	originalAgent := AgentSpec{Name: "claude", Config: AgentConfig{GlobalDir: "~/.claude/plugins/local"}}
	originalScope := agentcommon.InstallScopeGlobal
	originalPath := "/home/user/.claude/plugins/local/my-plugin"

	targets := []AgentTarget{
		{
			Agent:          originalAgent,
			Scope:          originalScope,
			DestinationDir: originalPath,
		},
	}

	result := InjectRepoKey(targets, "my-repo")

	assert.Equal(t, originalAgent.Name, result[0].Agent.Name)
	assert.Equal(t, originalScope, result[0].Scope)
	// Only DestinationDir should change for claude/codex
	assert.NotEqual(t, originalPath, result[0].DestinationDir)
	assert.Equal(t, filepath.Join("/home/user/.claude/plugins/local/my-repo", "my-plugin"), result[0].DestinationDir)
}
