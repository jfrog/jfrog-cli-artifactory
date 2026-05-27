package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportedAgentsList_SortedAndStable(t *testing.T) {
	first := SupportedAgentsList()
	for range 20 {
		assert.Equal(t, first, SupportedAgentsList())
	}
	parts := strings.Split(first, ", ")
	for i := 1; i < len(parts); i++ {
		assert.LessOrEqual(t, parts[i-1], parts[i])
	}
}

// withJfrogHome sets JFROG_CLI_HOME_DIR to a temp dir.
func withJfrogHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)
	return dir
}

func writeAgentConfig(t *testing.T, home, body string) {
	t.Helper()
	path := filepath.Join(home, "agents", "agent-config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLoadAgentRegistry_FallbackOnly(t *testing.T) {
	withJfrogHome(t)

	registry, err := LoadAgentRegistry()
	require.NoError(t, err)

	cursor, ok := registry["cursor"]
	require.True(t, ok)
	assert.False(t, cursor.FromConfig)
	assert.Equal(t, ".cursor/skills", cursor.Config.ProjectDir)
}

func TestLoadAgentRegistry_OverridesAndAdds(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{
		"agents": {
			"cursor": {"globalDir": "/abs/cursor", "projectDir": ".override/cursor"},
			"my-agent": {"globalDir": "~/.my/skills", "projectDir": ".my/skills"}
		}
	}`)

	registry, err := LoadAgentRegistry()
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
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `{"agents": {"broken": {}}}`)

	_, err := LoadAgentRegistry()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must define globalDir and/or projectDir")
}

func TestLoadAgentRegistry_RejectsBadJSON(t *testing.T) {
	home := withJfrogHome(t)
	writeAgentConfig(t, home, `not-json`)

	_, err := LoadAgentRegistry()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse agent config")
}

func TestParseHarnessList(t *testing.T) {
	got, err := ParseHarnessList("cursor, Claude-Code")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor", "claude-code"}, got)
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

func TestParseHarnessList_EmptyAndDuplicates(t *testing.T) {
	_, err := ParseHarnessList("")
	require.Error(t, err)

	_, err = ParseHarnessList("cursor,,claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty name")

	_, err = ParseHarnessList("cursor,cursor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestResolveAgent_Unknown(t *testing.T) {
	withJfrogHome(t)
	registry, err := LoadAgentRegistry()
	require.NoError(t, err)

	_, err = ResolveAgent(registry, "no-such-agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Supported agents")
	assert.Contains(t, err.Error(), "cursor")
}

func TestResolveAgentInstallDir_GlobalAndProject(t *testing.T) {
	withJfrogHome(t)
	globalDir := "/var/data/.cursor/skills"
	spec := AgentSpec{
		Name:   "cursor",
		Config: AgentConfig{GlobalDir: globalDir, ProjectDir: ".cursor/skills"},
	}

	wantGlobal, err := filepath.Abs(globalDir)
	require.NoError(t, err)

	abs, err := ResolveAgentInstallDir(spec, "", true)
	require.NoError(t, err)
	assert.Equal(t, wantGlobal, abs)

	projectRoot := t.TempDir()
	wantProject, err := filepath.Abs(filepath.Join(projectRoot, spec.Config.ProjectDir))
	require.NoError(t, err)
	abs, err = ResolveAgentInstallDir(spec, projectRoot, false)
	require.NoError(t, err)
	assert.Equal(t, wantProject, abs)
}
