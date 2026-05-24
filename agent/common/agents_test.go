package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPackageConfig = AgentPackageConfig{
	ConfigFileName: "agent-plugin-config.json",
	Defaults: map[string]AgentConfig{
		"cursor":      {GlobalDir: "~/.cursor/plugins", ProjectDir: ".cursor/plugins"},
		"claude-code": {GlobalDir: "~/.claude/plugins", ProjectDir: ".claude/plugins"},
	},
}

func withJfrogHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)
	return dir
}

func writePackageAgentConfig(t *testing.T, home, fileName, body string) {
	t.Helper()
	path := filepath.Join(home, "agents", fileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLoadAgentRegistry_FallbackOnly(t *testing.T) {
	withJfrogHome(t)

	registry, err := LoadAgentRegistry(testPackageConfig)
	require.NoError(t, err)
	cursor, ok := registry["cursor"]
	require.True(t, ok)
	assert.False(t, cursor.FromConfig)
	assert.Equal(t, ".cursor/plugins", cursor.Config.ProjectDir)
}

func TestLoadAgentRegistry_OverridesAndAdds(t *testing.T) {
	home := withJfrogHome(t)
	writePackageAgentConfig(t, home, testPackageConfig.ConfigFileName, `{
		"agents": {
			"cursor": {"globalDir": "/abs/cursor", "projectDir": ".override/cursor"},
			"my-agent": {"globalDir": "~/.my/plugins", "projectDir": ".my/plugins"}
		}
	}`)

	registry, err := LoadAgentRegistry(testPackageConfig)
	require.NoError(t, err)
	cursor := registry["cursor"]
	assert.True(t, cursor.FromConfig)
	assert.Equal(t, ".override/cursor", cursor.Config.ProjectDir)
	assert.Equal(t, "/abs/cursor", cursor.Config.GlobalDir)

	custom, ok := registry["my-agent"]
	require.True(t, ok)
	assert.True(t, custom.FromConfig)
	assert.Equal(t, ".my/plugins", custom.Config.ProjectDir)

	claude := registry["claude-code"]
	assert.False(t, claude.FromConfig)
	assert.Equal(t, ".claude/plugins", claude.Config.ProjectDir)
}

func TestLoadAgentRegistry_RejectsEmptyEntry(t *testing.T) {
	home := withJfrogHome(t)
	writePackageAgentConfig(t, home, testPackageConfig.ConfigFileName, `{"agents": {"broken": {}}}`)

	_, err := LoadAgentRegistry(testPackageConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must define globalDir and/or projectDir")
}

func TestLoadAgentRegistry_RejectsBadJSON(t *testing.T) {
	home := withJfrogHome(t)
	writePackageAgentConfig(t, home, testPackageConfig.ConfigFileName, `not-json`)

	_, err := LoadAgentRegistry(testPackageConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse agent config")
}

func TestParseAgentList(t *testing.T) {
	got, err := ParseAgentList("cursor, Claude-Code")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor", "claude-code"}, got)
}

func TestParseAgentList_EmptyAndDuplicates(t *testing.T) {
	_, err := ParseAgentList("")
	require.Error(t, err)

	_, err = ParseAgentList("cursor,,claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty name")

	_, err = ParseAgentList("cursor,cursor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestResolveAgent_Unknown(t *testing.T) {
	withJfrogHome(t)
	registry, err := LoadAgentRegistry(testPackageConfig)
	require.NoError(t, err)

	_, err = ResolveAgent(registry, "no-such-agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Supported agents")
	assert.Contains(t, err.Error(), "cursor")
}

func TestResolveAgentInstallDir_GlobalAndProject(t *testing.T) {
	globalDir := "/var/data/.cursor/plugins"
	spec := AgentSpec{
		Name:   "cursor",
		Config: AgentConfig{GlobalDir: globalDir, ProjectDir: ".cursor/plugins"},
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

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "x/y"), ExpandHome("~/x/y"))
	assert.Equal(t, "/abs/path", ExpandHome("/abs/path"))
	assert.Equal(t, "~", ExpandHome("~"))
}
