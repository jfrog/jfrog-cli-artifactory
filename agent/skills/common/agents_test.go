package common

import (
	"path/filepath"
	"strings"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportedAgentsList_SortedAndStable(t *testing.T) {
	first := agentcommon.SupportedAgentsList(Agents, agentcommon.SkillsAgentsKey)
	for range 20 {
		assert.Equal(t, first, agentcommon.SupportedAgentsList(Agents, agentcommon.SkillsAgentsKey))
	}
	parts := strings.Split(first, ", ")
	for i := 1; i < len(parts); i++ {
		assert.LessOrEqual(t, parts[i-1], parts[i])
	}
}

func TestLoadAgentRegistry_FallbackOnly(t *testing.T) {
	testutil.WithJfrogHome(t)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	require.NoError(t, err)

	cursor, ok := registry["cursor"]
	require.True(t, ok)
	assert.False(t, cursor.FromConfig)
	assert.Equal(t, ".cursor/skills", cursor.Config.ProjectDir)
}

func TestLoadAgentRegistry_OverridesAndAdds(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{
		"skills-agents": {
			"cursor": {"globalDir": "/abs/cursor", "projectDir": ".override/cursor"},
			"my-agent": {"globalDir": "~/.my/skills", "projectDir": ".my/skills"}
		}
	}`)

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	require.NoError(t, err)

	cursor := registry["cursor"]
	assert.True(t, cursor.FromConfig)
	assert.Equal(t, ".override/cursor", cursor.Config.ProjectDir)
	assert.Equal(t, "/abs/cursor", cursor.Config.GlobalDir)

	custom, ok := registry["my-agent"]
	require.True(t, ok)
	assert.True(t, custom.FromConfig)
	assert.Equal(t, ".my/skills", custom.Config.ProjectDir)

	claude := registry["claude-code"]
	assert.False(t, claude.FromConfig)
	assert.Equal(t, ".claude/skills", claude.Config.ProjectDir)
}

func TestLoadAgentRegistry_RejectsEmptyEntry(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `{"skills-agents": {"broken": {}}}`)

	_, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must define globalDir and/or projectDir")
}

func TestLoadAgentRegistry_RejectsBadJSON(t *testing.T) {
	home := testutil.WithJfrogHome(t)
	testutil.WriteAgentConfig(t, home, `not-json`)

	_, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse agent config")
}

func TestParseHarnessForList_Single(t *testing.T) {
	got, err := ParseHarnessForList("cursor")
	require.NoError(t, err)
	assert.Equal(t, "cursor", got)
}

func TestParseHarnessForList_RejectsCommaSeparated(t *testing.T) {
	_, err := ParseHarnessForList("cursor,claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one harness name")
}

func TestResolveAgent_Unknown(t *testing.T) {
	testutil.WithJfrogHome(t)
	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	require.NoError(t, err)

	_, err = agentcommon.ResolveAgent(registry, "no-such-agent", RegistryHelp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Supported agents")
	assert.Contains(t, err.Error(), "cursor")
}

func TestResolveAgentInstallDir_GlobalAndProject(t *testing.T) {
	testutil.WithJfrogHome(t)
	globalDir := "/var/data/.cursor/skills"
	spec := AgentSpec{
		Name:   "cursor",
		Config: AgentConfig{GlobalDir: globalDir, ProjectDir: ".cursor/skills"},
	}

	wantGlobal, err := filepath.Abs(globalDir)
	require.NoError(t, err)

	abs, err := agentcommon.ResolveAgentInstallDir(spec, "", true)
	require.NoError(t, err)
	assert.Equal(t, wantGlobal, abs)

	projectRoot := t.TempDir()
	wantProject, err := filepath.Abs(filepath.Join(projectRoot, spec.Config.ProjectDir))
	require.NoError(t, err)
	abs, err = agentcommon.ResolveAgentInstallDir(spec, projectRoot, false)
	require.NoError(t, err)
	assert.Equal(t, wantProject, abs)
}
