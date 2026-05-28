package install

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAgentTargetDirectories_ProjectScope(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := NewInstallCommand().
		SetSlug("my-skill").
		SetAgents([]common.AgentSpec{
			{Name: "cursor", Config: common.AgentConfig{ProjectDir: ".cursor/skills"}},
			{Name: "claude-code", Config: common.AgentConfig{ProjectDir: ".claude/skills"}},
		}).
		SetGlobal(false).
		SetProjectDir(projectRoot)

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, filepath.Join(projectRoot, ".cursor", "skills", "my-skill"), targets[0].DestinationDir)
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "skills", "my-skill"), targets[1].DestinationDir)
}

func TestResolveAgentTargetDirectories_GlobalScope(t *testing.T) {
	globalBase := filepath.Join(t.TempDir(), "global", ".cursor", "skills")
	wantBase, err := filepath.Abs(globalBase)
	require.NoError(t, err)

	cmd := NewInstallCommand().
		SetSlug("alpha").
		SetAgents([]common.AgentSpec{
			{Name: "cursor", Config: common.AgentConfig{GlobalDir: globalBase}},
		}).
		SetGlobal(true)

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(wantBase, "alpha"), targets[0].DestinationDir)
}

func TestResolveAgentTargetDirectories_LegacyInstallPath(t *testing.T) {
	tmp := t.TempDir()
	cmd := NewInstallCommand().SetSlug("legacy").SetInstallPath(tmp)
	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(tmp, "legacy"), targets[0].DestinationDir)
}

func TestCopyExtractedToTargets_WritesInstallManifest(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: x\nversion: 1.0.0\n---\n"), 0o644))
	dest := filepath.Join(t.TempDir(), "my-skill")
	projectRoot := t.TempDir()

	ic := NewInstallCommand().
		SetRepoKey("skills-repo").
		SetSlug("my-skill").
		SetVersion("1.2.3").
		SetProjectDir(projectRoot)

	targets := []common.AgentTarget{{
		Agent:          common.AgentSpec{Name: "cursor"},
		DestinationDir: dest,
		Scope:          common.ScopeProject,
	}}
	rows := ic.CopyExtractedToTargets(src, targets)
	require.Len(t, rows, 1)
	assert.Equal(t, agentcommon.SummaryStatusOK, rows[0].Status)

	got, err := agentcommon.ReadInstallInfoManifest(dest, common.SkillInfoManifestFile)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "skills-repo", got.Repo)
	assert.Equal(t, "my-skill", got.Slug)
	assert.Equal(t, "1.2.3", got.InstalledVersion)
	assert.Equal(t, "project", got.Scope)
	assert.Equal(t, "cursor", got.Agent)
	assert.Equal(t, projectRoot, got.ProjectDir)
}
