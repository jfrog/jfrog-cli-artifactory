package update

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReserveUpdateBackupPath(t *testing.T) {
	base := t.TempDir()
	p, err := reserveUpdateBackupPath(base, "skill-a")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, skillBackupDirName), filepath.Dir(p))
	assert.Contains(t, filepath.Base(p), "skill-a-backup-")
	_, err = os.Stat(p)
	require.True(t, errors.Is(err, fs.ErrNotExist), "reserved path must not exist until rename")
}

func TestPreUpdateTargets_NotInstalled(t *testing.T) {
	base := t.TempDir()
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: filepath.Join(base, "missing"),
	}
	checks := preUpdateTargets([]common.AgentTarget{target}, "1.0.0", false, true)
	require.Len(t, checks, 1)
	assert.Contains(t, checks[0].failureReason, "not installed")
}

func TestPreUpdateTargets_UpToDate(t *testing.T) {
	dir := skillDir(t, "---\nname: web\nversion: 2.0.0\n---\n")
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]common.AgentTarget{target}, "2.0.0", false, true)
	require.Len(t, checks, 1)
	assert.True(t, checks[0].alreadyAtTargetVersion)
	assert.Equal(t, "2.0.0", checks[0].installedVersion)
}

func TestPreUpdateTargets_UpToDate_UsesManifestVersion(t *testing.T) {
	dir := skillDir(t, "---\nname: web\nversion: 1.0.0\n---\n")
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, common.SkillInfoManifestFile, common.SkillInfoManifest{
		Repo:             "r",
		Slug:             "web",
		InstalledVersion: "2.0.0",
		Scope:            "project",
		Agent:            "cursor",
	}))
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]common.AgentTarget{target}, "2.0.0", false, true)
	require.Len(t, checks, 1)
	assert.True(t, checks[0].alreadyAtTargetVersion)
	assert.Equal(t, "2.0.0", checks[0].installedVersion)
}

func TestPreUpdateTargets_ForceOverridesUpToDate(t *testing.T) {
	dir := skillDir(t, "---\nname: web\nversion: 2.0.0\n---\n")
	target := common.AgentTarget{
		Agent:          common.AgentSpec{Name: "cursor"},
		Scope:          common.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]common.AgentTarget{target}, "2.0.0", true, true)
	require.Len(t, checks, 1)
	assert.False(t, checks[0].alreadyAtTargetVersion)
}

func TestInitialResultsAndUpdatable_Mixed(t *testing.T) {
	checks := []preUpdate{
		{agentTarget: common.AgentTarget{Agent: common.AgentSpec{Name: "a1"}, Scope: common.ScopeProject, DestinationDir: "/x/a1"}, failureReason: "not installed"},
		{agentTarget: common.AgentTarget{Agent: common.AgentSpec{Name: "a2"}, Scope: common.ScopeProject, DestinationDir: "/x/a2"}, alreadyAtTargetVersion: true, installedVersion: "1.0.0"},
		{agentTarget: common.AgentTarget{Agent: common.AgentSpec{Name: "a3"}, Scope: common.ScopeProject, DestinationDir: "/x/a3"}, installedVersion: "1.0.0"},
	}
	results, updatable := initialResultsAndUpdatable(checks, "2.0.0")
	require.Len(t, results, 2)
	require.Len(t, updatable, 1)
	assert.Equal(t, agentcommon.SummaryStatusFailed, results[0].Status)
	assert.Equal(t, agentcommon.SummaryStatusSkipped, results[1].Status)
	assert.Equal(t, "a3", updatable[0].agentTarget.Agent.Name)
}

func TestFinalError_AllOK(t *testing.T) {
	results := []agentcommon.SummaryRow{
		{Status: agentcommon.SummaryStatusOK},
		{Status: agentcommon.SummaryStatusSkipped},
	}
	require.NoError(t, finalError(results))
}

func TestFinalError_PartialSuccess(t *testing.T) {
	results := []agentcommon.SummaryRow{
		{Status: agentcommon.SummaryStatusFailed},
		{Status: agentcommon.SummaryStatusOK},
	}
	require.NoError(t, finalError(results))
}

func TestFinalError_AllFailed(t *testing.T) {
	results := []agentcommon.SummaryRow{
		{Status: agentcommon.SummaryStatusFailed},
		{Status: agentcommon.SummaryStatusFailed},
	}
	err := finalError(results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed for all targets")
}

func TestUpdateOneSkill_SuccessRemovesBackup(t *testing.T) {
	dir := skillDir(t, "---\nname: web\nversion: 1.0.0\n---\n")
	check := preUpdate{
		agentTarget: common.AgentTarget{
			Agent:          common.AgentSpec{Name: "cursor"},
			Scope:          common.ScopeProject,
			DestinationDir: dir,
		},
		installedVersion: "1.0.0",
	}

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: web\nversion: 2.0.0\n---\n"), 0o644))

	installCommand := install.NewInstallCommand()
	row := updateOneSkill(src, installCommand, check)
	assert.Equal(t, agentcommon.SummaryStatusOK, row.Status)
	assert.Equal(t, agentcommon.SummaryDetailOKInstall, row.Detail)

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"web"}, names)

	backupRoot := filepath.Join(filepath.Dir(dir), skillBackupDirName)
	_, statErr := os.Stat(backupRoot)
	require.Error(t, statErr)
	assert.True(t, os.IsNotExist(statErr), skillBackupDirName+" should be removed when empty after successful update")
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "2.0.0")
}

func TestRunUpdate_AllRejectsSlugFlag(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "cursor", "slug": "web"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --slug")
}

func TestRunUpdate_AllRejectsPositionalArg(t *testing.T) {
	ctx := newUpdateContext(t, []string{"web"}, map[string]string{"harness": "cursor"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected positional argument")
}

func TestRunUpdate_AllRejectsVersion(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "cursor", "version": "1.2.3"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --version")
}

func TestRunUpdate_AllRejectsPath(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"path": t.TempDir()}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --path")
}

func TestRunUpdate_RequiresSlugWithoutAll(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "cursor"}, nil)
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestRunUpdate_RejectsPositionalSlug(t *testing.T) {
	ctx := newUpdateContext(t, []string{"web"}, map[string]string{"harness": "cursor"}, nil)
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use --slug")
}

func TestDiscoverInstalledSkillTargets_MergesHarnesses(t *testing.T) {
	projectRoot := t.TempDir()
	skillsDir := ".cursor/skills"
	installRoot := filepath.Join(projectRoot, skillsDir)

	dir := filepath.Join(installRoot, "shared")
	require.NoError(t, os.MkdirAll(dir, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: shared\nversion: 1.0.0\n---\n"), agentcommon.DefaultFileMode))

	flags := agentcommon.InstallFlagsResult{
		Specs: []common.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: skillsDir}},
			{Name: "claude", Config: agentcommon.AgentConfig{ProjectDir: skillsDir}},
		},
		ProjectDirAbs: projectRoot,
	}

	slugOrder, slugToTargets, err := discoverInstalledSkillTargets(flags)
	require.NoError(t, err)
	require.Equal(t, []string{"shared"}, slugOrder)
	require.Len(t, slugToTargets["shared"], 2)
}

func TestDiscoverInstalledSkillTargets_SkillMdOnly(t *testing.T) {
	projectRoot := t.TempDir()
	skillsDir := ".cursor/skills"
	installRoot := filepath.Join(projectRoot, skillsDir)
	dir := filepath.Join(installRoot, "legacy")
	require.NoError(t, os.MkdirAll(dir, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: legacy\nversion: 0.9.0\n---\n"), agentcommon.DefaultFileMode))

	flags := agentcommon.InstallFlagsResult{
		Specs:         []common.AgentSpec{{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: skillsDir}}},
		ProjectDirAbs: projectRoot,
	}

	slugOrder, _, err := discoverInstalledSkillTargets(flags)
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy"}, slugOrder)
}

func TestFinalizeUpdateAll_AllFailed(t *testing.T) {
	combined := []agentcommon.UpdateAllSummaryRow{
		{Agent: "cursor", Name: "a", Status: agentcommon.SummaryStatusFailed},
	}
	outcome := updateAllOutcome{anyFailed: true}
	err := finalizeUpdateAll(combined, outcome, "table")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed for all targets")
}

func TestConfirmUpdateAll_AbortsWhenUserDeclines(t *testing.T) {
	oldAsk := askYesNo
	oldCheck := isNonInteractive
	defer func() {
		askYesNo = oldAsk
		isNonInteractive = oldCheck
	}()
	isNonInteractive = func() bool { return false }
	askYesNo = func(_ string, _ bool) bool { return false }

	err := confirmUpdateAll(update{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aborted by user")
}

func newUpdateContext(t *testing.T, args []string, stringFlags map[string]string, boolFlags map[string]bool) *components.Context {
	t.Helper()
	ctx := &components.Context{Arguments: args}
	ctx.PrintCommandHelp = func(string) error { return nil }
	for name, value := range stringFlags {
		ctx.AddStringFlag(name, value)
	}
	for name, value := range boolFlags {
		ctx.AddBoolFlag(name, value)
	}
	return ctx
}

func skillDir(t *testing.T, skillMD string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "web")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644))
	return dir
}
