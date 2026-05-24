package install

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInstallTestContext() *components.Context {
	ctx := &components.Context{}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

func TestResolveInstallTargets_ProjectScope(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := NewInstallCommand().
		SetSlug("my-plugin").
		SetAgents([]agentcommon.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: ".cursor/plugins"}},
			{Name: "claude-code", Config: agentcommon.AgentConfig{ProjectDir: ".claude/plugins"}},
		}).
		SetGlobal(false).
		SetProjectDir(projectRoot)

	targets, err := cmd.resolveInstallTargets()
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, filepath.Join(projectRoot, ".cursor", "plugins", "my-plugin"), targets[0].DestinationDir)
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "plugins", "my-plugin"), targets[1].DestinationDir)
}

func TestResolveInstallTargets_GlobalScope(t *testing.T) {
	globalBase := filepath.Join(t.TempDir(), "global", ".cursor", "plugins")
	wantBase, err := filepath.Abs(globalBase)
	require.NoError(t, err)

	cmd := NewInstallCommand().
		SetSlug("alpha").
		SetAgents([]agentcommon.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{GlobalDir: globalBase}},
		}).
		SetGlobal(true)

	targets, err := cmd.resolveInstallTargets()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(wantBase, "alpha"), targets[0].DestinationDir)
}

func TestResolveInstallTargets_PathMode(t *testing.T) {
	base := t.TempDir()
	cmd := NewInstallCommand().SetSlug("legacy").SetInstallPath(base)

	targets, err := cmd.resolveInstallTargets()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(base, "legacy"), targets[0].DestinationDir)
	assert.Equal(t, agentcommon.ScopePath, targets[0].Scope)
}

func TestResolveInstallTargets_ProjectMissingDir(t *testing.T) {
	cmd := NewInstallCommand().
		SetSlug("x").
		SetAgents([]agentcommon.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: ".cursor/plugins"}},
		}).
		SetGlobal(false)

	_, err := cmd.resolveInstallTargets()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project directory is required")
}

func TestCopyExtractedToTargets_CopiesFiles(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.json"), []byte(`{"name":"x"}`), 0o644))

	dest := filepath.Join(t.TempDir(), "my-plugin")
	cmd := NewInstallCommand().SetSlug("my-plugin").SetVersion("1.2.3")
	targets := []agentcommon.AgentTarget{{
		Agent:          agentcommon.AgentSpec{Name: "cursor"},
		DestinationDir: dest,
		Scope:          agentcommon.ScopeProject,
	}}

	rows := cmd.copyExtractedToTargets(src, targets)
	require.Len(t, rows, 1)
	assert.Equal(t, agentcommon.SummaryStatusOK, rows[0].Status)

	data, err := os.ReadFile(filepath.Join(dest, "plugin.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"name":"x"}`, string(data))
}

func TestCopyExtractedToTargets_FileAtDestinationFails(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.json"), []byte("{}"), 0o644))

	parent := t.TempDir()
	dest := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(dest, []byte("hi"), 0o644))

	cmd := NewInstallCommand().SetSlug("blocker")
	rows := cmd.copyExtractedToTargets(src, []agentcommon.AgentTarget{{
		Agent:          agentcommon.AgentSpec{Name: "cursor"},
		DestinationDir: dest,
		Scope:          agentcommon.ScopeProject,
	}})
	require.Len(t, rows, 1)
	assert.Equal(t, agentcommon.SummaryStatusFailed, rows[0].Status)
}

func TestRun_NoAgentsAndNoPath(t *testing.T) {
	err := NewInstallCommand().SetSlug("x").Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent is required")
}

func TestRunInstall_NoArgs(t *testing.T) {
	c := newInstallTestContext()
	err := RunInstall(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestRunInstall_InvalidSlug(t *testing.T) {
	c := newInstallTestContext()
	c.Arguments = []string{"Bad Slug"}
	err := RunInstall(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plugin slug")
}
