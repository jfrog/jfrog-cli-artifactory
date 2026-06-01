package update

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/install"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReserveUpdateBackupPath(t *testing.T) {
	base := t.TempDir()
	p, err := reserveUpdateBackupPath(base, "plugin-a")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, pluginBackupDirName), filepath.Dir(p))
	assert.Contains(t, filepath.Base(p), "plugin-a-backup-")
	_, err = os.Stat(p)
	require.True(t, errors.Is(err, fs.ErrNotExist), "reserved path must not exist until rename")
}

func TestPreUpdateTargets_NotInstalled(t *testing.T) {
	base := t.TempDir()
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: filepath.Join(base, "missing"),
	}
	checks := preUpdateTargets([]plugincommon.AgentTarget{target}, "1.0.0", false, true)
	require.Len(t, checks, 1)
	assert.Contains(t, checks[0].failureReason, "not installed")
}

func TestPreUpdateTargets_UpToDate(t *testing.T) {
	dir := pluginDir(t, `{"name":"web","version":"2.0.0"}`)
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]plugincommon.AgentTarget{target}, "2.0.0", false, true)
	require.Len(t, checks, 1)
	assert.True(t, checks[0].alreadyAtTargetVersion)
	assert.Equal(t, "2.0.0", checks[0].installedVersion)
}

func TestPreUpdateTargets_UpToDate_UsesManifestVersion(t *testing.T) {
	dir := pluginDir(t, `{"name":"web","version":"1.0.0"}`)
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, plugincommon.PluginInfoManifestFile, plugincommon.PluginInfoManifest{
		Repo:             "r",
		Slug:             "web",
		InstalledVersion: "2.0.0",
		Scope:            "project",
		Agent:            "claude",
	}))
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]plugincommon.AgentTarget{target}, "2.0.0", false, true)
	require.Len(t, checks, 1)
	assert.True(t, checks[0].alreadyAtTargetVersion)
	assert.Equal(t, "2.0.0", checks[0].installedVersion)
}

func TestPreUpdateTargets_ForceOverridesUpToDate(t *testing.T) {
	dir := pluginDir(t, `{"name":"web","version":"2.0.0"}`)
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}
	checks := preUpdateTargets([]plugincommon.AgentTarget{target}, "2.0.0", true, true)
	require.Len(t, checks, 1)
	assert.False(t, checks[0].alreadyAtTargetVersion)
}

func TestInitialResultsAndUpdatable_Mixed(t *testing.T) {
	checks := []preUpdate{
		{agentTarget: plugincommon.AgentTarget{Agent: plugincommon.AgentSpec{Name: "a1"}, Scope: plugincommon.ScopeProject, DestinationDir: "/x/a1"}, failureReason: "not installed"},
		{agentTarget: plugincommon.AgentTarget{Agent: plugincommon.AgentSpec{Name: "a2"}, Scope: plugincommon.ScopeProject, DestinationDir: "/x/a2"}, alreadyAtTargetVersion: true, installedVersion: "1.0.0"},
		{agentTarget: plugincommon.AgentTarget{Agent: plugincommon.AgentSpec{Name: "a3"}, Scope: plugincommon.ScopeProject, DestinationDir: "/x/a3"}, installedVersion: "1.0.0"},
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

func TestUpdateOnePlugin_SuccessRemovesBackup(t *testing.T) {
	dir := pluginDir(t, `{"name":"web","version":"1.0.0"}`)
	check := preUpdate{
		agentTarget: plugincommon.AgentTarget{
			Agent:          plugincommon.AgentSpec{Name: "claude"},
			Scope:          plugincommon.ScopeProject,
			DestinationDir: dir,
		},
		installedVersion: "1.0.0",
	}

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.json"), []byte(`{"name":"web","version":"2.0.0"}`), 0o644))

	installCommand := install.NewInstallCommand().SetSlug("web").SetVersion("2.0.0").SetRepoKey("r")
	row := updateOnePlugin(src, installCommand, check)
	assert.Equal(t, agentcommon.SummaryStatusOK, row.Status)
	assert.Equal(t, agentcommon.SummaryDetailOKInstall, row.Detail)

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"web"}, names)

	backupRoot := filepath.Join(filepath.Dir(dir), pluginBackupDirName)
	_, statErr := os.Stat(backupRoot)
	require.Error(t, statErr)
	assert.True(t, os.IsNotExist(statErr), pluginBackupDirName+" should be removed when empty after successful update")
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "2.0.0")
}

func TestResolveTargetVersion_ExplicitUsedDirectly(t *testing.T) {
	got, err := resolveTargetVersion(nil, "repo", "slug", "1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", got)
}

func TestResolveTargetVersion_RejectsInvalid(t *testing.T) {
	_, err := resolveTargetVersion(nil, "repo", "slug", "not-a-version")
	require.Error(t, err)
}

func TestRunUpdate_AllRejectsSlugArg(t *testing.T) {
	ctx := newUpdateContext(t, []string{"web"}, map[string]string{"harness": "claude"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with a slug argument")
}

func TestRunUpdate_AllRejectsVersion(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "claude", "version": "1.2.3"}, map[string]bool{"all": true})
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

func TestDiscoverInstalledPluginTargets_MergesHarnesses(t *testing.T) {
	projectRoot := t.TempDir()
	pluginsDir := "plugins"
	installRoot := filepath.Join(projectRoot, pluginsDir)

	require.NoError(t, os.MkdirAll(filepath.Join(installRoot, "shared"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(installRoot, "shared", "plugin.json"), []byte(`{"name":"shared","version":"1.0.0"}`), 0o644))

	flags := agentcommon.InstallFlagsResult{
		Specs: []plugincommon.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}},
			{Name: "claude", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}},
		},
		ProjectDirAbs: projectRoot,
	}

	slugOrder, slugToTargets, err := discoverInstalledPluginTargets(flags)
	require.NoError(t, err)
	require.Equal(t, []string{"shared"}, slugOrder)
	require.Len(t, slugToTargets["shared"], 2)
}

func TestDiscoverInstalledPluginTargets_PluginJSONOnly(t *testing.T) {
	projectRoot := t.TempDir()
	pluginsDir := "plugins"
	installRoot := filepath.Join(projectRoot, pluginsDir)
	pluginDir := filepath.Join(installRoot, "legacy")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"name":"legacy","version":"0.9.0"}`), 0o644))

	flags := agentcommon.InstallFlagsResult{
		Specs:         []plugincommon.AgentSpec{{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}}},
		ProjectDirAbs: projectRoot,
	}

	slugOrder, _, err := discoverInstalledPluginTargets(flags)
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

func TestFinalizeUpdateAll_ReturnsFirstResolveErrWhenNothingUpdated(t *testing.T) {
	resolveErr := errors.New("no versions in repo")
	outcome := updateAllOutcome{firstResolveErr: resolveErr}
	err := finalizeUpdateAll(nil, outcome, "table")
	require.Error(t, err)
	assert.ErrorIs(t, err, resolveErr)
}

func TestApplyUpdateAllForSlugs_ContinuesOnDownloadError(t *testing.T) {
	oldResolve := resolveLatestPluginVersion
	oldUpdate := updateSlugAcrossTargetsFn
	defer func() {
		resolveLatestPluginVersion = oldResolve
		updateSlugAcrossTargetsFn = oldUpdate
	}()

	resolveLatestPluginVersion = func(*config.ServerDetails, string, string) (string, error) {
		return "2.0.0", nil
	}
	updateSlugAcrossTargetsFn = func(opts updateOptions, slug, targetVersion string, targets []plugincommon.AgentTarget) ([]agentcommon.SummaryRow, error) {
		if slug == "bad" {
			return nil, errors.New("download failed")
		}
		return []agentcommon.SummaryRow{
			{Agent: targets[0].Agent.Name, Scope: string(targets[0].Scope), Path: targets[0].DestinationDir, Status: agentcommon.SummaryStatusOK},
		}, nil
	}

	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "cursor"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: "/tmp/bad",
	}
	opts := updateOptions{serverDetails: &config.ServerDetails{}, repoKey: "repo"}
	combined, outcome := applyUpdateAllForSlugs(opts, []string{"bad", "good"}, map[string][]plugincommon.AgentTarget{
		"bad":  {target},
		"good": {{Agent: plugincommon.AgentSpec{Name: "cursor"}, Scope: plugincommon.ScopeProject, DestinationDir: "/tmp/good"}},
	})

	require.Equal(t, 2, len(combined))
	assert.Equal(t, agentcommon.SummaryStatusFailed, combined[0].Status)
	assert.Equal(t, agentcommon.SummaryStatusOK, combined[1].Status)
	assert.True(t, outcome.anyOK)
	assert.True(t, outcome.anyFailed)
	assert.Equal(t, 2, outcome.updatedSlugCount)
}

func TestMovePluginAsideForUpdate_MissingInstallDir(t *testing.T) {
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "cursor"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: filepath.Join(t.TempDir(), "missing"),
	}
	_, err := movePluginAsideForUpdate(target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "move current plugin aside")
}

func TestRestorePluginFromBackup_RestoresPreviousInstall(t *testing.T) {
	parent := t.TempDir()
	live := filepath.Join(parent, "web")
	backup := filepath.Join(parent, ".plugin-backup", "web-backup-test")

	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "plugin.json"), []byte(`{"name":"web","version":"1.0.0"}`), 0o644))
	require.NoError(t, os.MkdirAll(backup, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(backup, "plugin.json"), []byte(`{"name":"web","version":"0.9.0"}`), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(live, "failed-copy"), 0o755))

	target := plugincommon.AgentTarget{DestinationDir: live}
	require.NoError(t, restorePluginFromBackup(target, backup))

	data, err := os.ReadFile(filepath.Join(live, "plugin.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "0.9.0")
	_, err = os.Stat(backup)
	require.True(t, os.IsNotExist(err))
}

func TestRunUpdate_RequiresArgWithoutAll(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "claude"}, nil)
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
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

func pluginDir(t *testing.T, manifest string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "web")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644))
	return dir
}
