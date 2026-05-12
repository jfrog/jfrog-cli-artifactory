package update

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInstalledVersion_Found(t *testing.T) {
	dir := skillDir(t, "web-search", `---
name: web-search
version: 1.2.3
---
`)
	version, err := readInstalledVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", version)
}

func TestReadInstalledVersion_NoVersionField(t *testing.T) {
	dir := skillDir(t, "web-search", `---
name: web-search
description: No version here
---
`)
	version, err := readInstalledVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "", version, "empty version when SKILL.md has no version field")
}

func TestReadInstalledVersion_NotInstalled(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "nonexistent-skill")

	_, err := readInstalledVersion(missing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "missing SKILL.md must surface fs.ErrNotExist for preflight")
}

func TestValidateExistingDir_NotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	err := common.ValidateExistingDir(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestReadInstalledVersion_InvalidFrontmatter(t *testing.T) {
	dir := skillDir(t, "bad-skill", "# No frontmatter at all\n")
	_, err := readInstalledVersion(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse SKILL.md")
}

func TestReserveUpdateBackupPath(t *testing.T) {
	base := t.TempDir()
	p, err := reserveUpdateBackupPath(base, "skill-a")
	require.NoError(t, err)
	assert.Contains(t, p, ".skill-a.jfrog-update-backup-")
	_, statErr := os.Stat(p)
	require.True(t, errors.Is(statErr, fs.ErrNotExist), "reserved path must not exist until rename")
}

func TestPreflightTargets_NotInstalled(t *testing.T) {
	base := t.TempDir()
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: filepath.Join(base, "missing"),
	}
	checks := preflightTargets([]common.AgentTarget{target}, "1.0.0", false, true)
	require.Len(t, checks, 1)
	assert.Contains(t, checks[0].failure, "not installed")
}

func TestPreflightTargets_UpToDate(t *testing.T) {
	dir := skillDir(t, "web", "---\nname: web\nversion: 2.0.0\n---\n")
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: dir,
	}
	checks := preflightTargets([]common.AgentTarget{target}, "2.0.0", false, true)
	require.Len(t, checks, 1)
	assert.True(t, checks[0].upToDate)
	assert.Equal(t, "2.0.0", checks[0].installedVer)
}

func TestPreflightTargets_ForceOverridesUpToDate(t *testing.T) {
	dir := skillDir(t, "web", "---\nname: web\nversion: 2.0.0\n---\n")
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: dir,
	}
	checks := preflightTargets([]common.AgentTarget{target}, "2.0.0", true, true)
	require.Len(t, checks, 1)
	assert.False(t, checks[0].upToDate)
}

func TestInitialResultsAndUpdatable_Mixed(t *testing.T) {
	checks := []preflight{
		{target: common.AgentTarget{Agent: common.AgentSpec{Name: "a1"}, Scope: common.ScopeProject, DestinationDir: "/x/a1"}, failure: "not installed"},
		{target: common.AgentTarget{Agent: common.AgentSpec{Name: "a2"}, Scope: common.ScopeProject, DestinationDir: "/x/a2"}, upToDate: true, installedVer: "1.0.0"},
		{target: common.AgentTarget{Agent: common.AgentSpec{Name: "a3"}, Scope: common.ScopeProject, DestinationDir: "/x/a3"}, installedVer: "1.0.0"},
	}
	results, updatable := initialResultsAndUpdatable(checks, "2.0.0")
	require.Len(t, results, 2)
	require.Len(t, updatable, 1)
	assert.Equal(t, install.SummaryStatusFailed, results[0].Status)
	assert.Equal(t, install.SummaryStatusUpToDate, results[1].Status)
	assert.Equal(t, "a3", updatable[0].target.Agent.Name)
}

func TestFinalError_AllOK(t *testing.T) {
	results := []install.SummaryRow{
		{Status: install.SummaryStatusOK},
		{Status: install.SummaryStatusUpToDate},
	}
	require.NoError(t, finalError(results))
}

func TestFinalError_PartialSuccess(t *testing.T) {
	results := []install.SummaryRow{
		{Status: install.SummaryStatusFailed},
		{Status: install.SummaryStatusOK},
	}
	require.NoError(t, finalError(results))
}

func TestFinalError_AllFailed(t *testing.T) {
	results := []install.SummaryRow{
		{Status: install.SummaryStatusFailed},
		{Status: install.SummaryStatusFailed},
	}
	err := finalError(results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed for all targets")
}

func TestUpdateOneWithPrepared_SuccessRemovesBackup(t *testing.T) {
	dir := skillDir(t, "web", "---\nname: web\nversion: 1.0.0\n---\n")
	check := preflight{
		target: common.AgentTarget{
			Agent:          common.AgentSpec{Name: "cursor"},
			Scope:          common.ScopeProject,
			DestinationDir: dir,
		},
		installedVer: "1.0.0",
	}

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: web\nversion: 2.0.0\n---\n"), 0o644))

	ic := install.NewInstallCommand()
	row := updateOneWithPrepared(src, ic, check)
	assert.Equal(t, install.SummaryStatusOK, row.Status)
	assert.Equal(t, install.SummaryDetailOKInstall, row.Detail)

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "web", entries[0].Name())
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "2.0.0")
}

func skillDir(t *testing.T, slug, skillMD string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644))
	return dir
}
