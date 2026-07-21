package update

import (
	"errors"
	"fmt"
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

func init() {
	// Prevent real agent CLI binaries from being invoked during unit tests, and make
	// results independent of whether claude/codex happen to be installed on the
	// machine running the tests (e.g. CI runners don't have them on PATH).
	plugincommon.ClaudeExec = func(_ ...string) error { return nil }
	plugincommon.CodexExec = func(_ ...string) error { return nil }
	plugincommon.LookPathClaude = func() (string, error) { return "/usr/bin/claude", nil }
	plugincommon.LookPathCodex = func() (string, error) { return "/usr/bin/codex", nil }
	// Prevent tests from depending on the real machine's ~/.claude and ~/.codex
	// state; tests that care about this check override it themselves.
	isRegisteredWithNativeAgent = func(_, _, _ string) (bool, error) { return true, nil }
}

func TestReserveUpdateBackupPath(t *testing.T) {
	base := t.TempDir()
	reservedBackupPath, err := reserveUpdateBackupPath(base, "plugin-a")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, pluginBackupDirName), filepath.Dir(reservedBackupPath))
	assert.Contains(t, filepath.Base(reservedBackupPath), "plugin-a-backup-")
	_, err = os.Stat(reservedBackupPath)
	require.True(t, errors.Is(err, fs.ErrNotExist), "reserved path must not exist until rename")
}

func TestPreUpdateTargets_NotInstalled(t *testing.T) {
	base := t.TempDir()
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: filepath.Join(base, "missing"),
	}
	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "1.0.0", false, true, false)
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
	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", false, true, false)
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
	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", false, true, false)
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
	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", true, true, false)
	require.Len(t, checks, 1)
	assert.False(t, checks[0].alreadyAtTargetVersion)
}

func TestPreUpdateTargets_NotRegisteredNatively(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(agentName, slug, repoKey string) (bool, error) {
		assert.Equal(t, "claude", agentName)
		assert.Equal(t, "web", slug)
		assert.Equal(t, "repo", repoKey)
		return false, nil
	}
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	dir := pluginDir(t, `{"name":"web","version":"2.0.0"}`)
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}

	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", false, true, false)
	require.Len(t, checks, 1)
	assert.Contains(t, checks[0].failureReason, "not installed for claude")
	assert.Contains(t, checks[0].failureReason, "native plugin registry")
	assert.False(t, checks[0].alreadyAtTargetVersion, "should report as not-installed, not as already up to date")
}

func TestPreUpdateTargets_NativeCheckErrorFallsBackToFileRecord(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(_, _, _ string) (bool, error) {
		return false, errors.New("could not parse config.toml")
	}
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	dir := pluginDir(t, `{"name":"web","version":"2.0.0"}`)
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "codex"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}

	// A native-check error must not block the update; it should fall back to
	// jf's own file-based record (already at 2.0.0 here).
	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", false, true, false)
	require.Len(t, checks, 1)
	assert.Empty(t, checks[0].failureReason)
	assert.True(t, checks[0].alreadyAtTargetVersion)
}

func TestPreUpdateTargets_SkipNativeCheckIgnoresNativeUnregistration(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(_, _, _ string) (bool, error) {
		t.Fatal("isRegisteredWithNativeAgent must not be called when skipNativeCheck is true")
		return false, nil
	}
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	// Simulates --all: the plugin was uninstalled natively (would normally fail the
	// native-registry check), but jf's own file record still shows it installed at 2.0.0.
	dir := pluginDir(t, `{"name":"web","version":"2.0.0"}`)
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "claude"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: dir,
	}

	checks := preUpdateTargets("web", "repo", []plugincommon.AgentTarget{target}, "2.0.0", false, true, true)
	require.Len(t, checks, 1)
	assert.Empty(t, checks[0].failureReason)
	assert.True(t, checks[0].alreadyAtTargetVersion)
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
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.json"), []byte(`{"name":"web","version":"2.0.0"}`), agentcommon.DefaultFileMode))

	installCommand := install.NewInstallCommand().SetSlug("web").SetVersion("2.0.0").SetRepoKey("r")
	row := updatePlugin(src, installCommand, check)
	assert.Equal(t, agentcommon.SummaryStatusOK, row.Status)
	assert.Equal(t, agentcommon.SummaryDetailOKUpdate, row.Detail)

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// .claude-plugin/ is created by the Claude post-install hook (marketplace.json).
	assert.ElementsMatch(t, []string{".claude-plugin", "web"}, names)

	backupRoot := filepath.Join(filepath.Dir(dir), pluginBackupDirName)
	_, statErr := os.Stat(backupRoot)
	require.Error(t, statErr)
	assert.True(t, os.IsNotExist(statErr), pluginBackupDirName+" should be removed when empty after successful update")
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "2.0.0")
}

func TestResolveTargetVersion_ExplicitUsedDirectly(t *testing.T) {
	restore := resolvePluginVersion
	resolvePluginVersion = func(_ *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
		assert.Equal(t, "repo", repoKey)
		assert.Equal(t, "slug", slug)
		assert.Equal(t, "1.2.3", requested)
		assert.True(t, quiet)
		return "1.2.3", nil
	}
	t.Cleanup(func() { resolvePluginVersion = restore })

	got, err := resolveTargetVersion(nil, "repo", "slug", "1.2.3", true)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", got)
}

func TestResolveTargetVersion_RejectsInvalid(t *testing.T) {
	_, err := resolveTargetVersion(nil, "repo", "slug", "not-a-version", true)
	require.Error(t, err)
}

func TestDiagnoseNotFoundError_PassesThroughUnrelatedErrors(t *testing.T) {
	original := errors.New("some other failure")
	got := diagnoseNotFoundError(original, "web", "repo", nil)
	assert.Same(t, original, got, "non-not-found errors must be returned unchanged")
}

func TestDiagnoseNotFoundError_PassesThroughWhenGenuinelyMissing(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(_, _, _ string) (bool, error) { return false, nil }
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	notFound := fmt.Errorf("plugin '%s' %w '%s'", "web", plugincommon.ErrPluginNotFoundInRepo, "repo")
	target := plugincommon.AgentTarget{Agent: plugincommon.AgentSpec{Name: "claude"}, DestinationDir: filepath.Join(t.TempDir(), "missing")}

	got := diagnoseNotFoundError(notFound, "web", "repo", []plugincommon.AgentTarget{target})
	assert.Same(t, notFound, got, "when not registered anywhere, the plugin genuinely doesn't exist — keep the original message")
}

func TestDiagnoseNotFoundError_EnrichedWhenInstalledNativelyButNotByJF(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(agentName, slug, repoKey string) (bool, error) {
		assert.Equal(t, "security-guidance", slug)
		assert.Equal(t, "claude-plugins-official", repoKey)
		return agentName == "claude", nil
	}
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	notFound := fmt.Errorf("plugin '%s' %w '%s'", "security-guidance", plugincommon.ErrPluginNotFoundInRepo, "claude-plugins-official")
	targets := []plugincommon.AgentTarget{
		{Agent: plugincommon.AgentSpec{Name: "claude"}, DestinationDir: filepath.Join(t.TempDir(), "claude-missing")},
		{Agent: plugincommon.AgentSpec{Name: "codex"}, DestinationDir: filepath.Join(t.TempDir(), "codex-missing")},
	}

	got := diagnoseNotFoundError(notFound, "security-guidance", "claude-plugins-official", targets)
	require.Error(t, got)
	assert.Contains(t, got.Error(), "installed for claude")
	assert.Contains(t, got.Error(), "not via jfrog-cli")
	assert.NotContains(t, got.Error(), "codex", "codex was not reported as natively registered, so it must not be listed")
}

func TestDiagnoseNotFoundError_NotEnrichedWhenJFLocalCopyExists(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(_, _, _ string) (bool, error) { return true, nil }
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	// jf's own local copy exists here (plugin.json present) even though version lookup failed —
	// e.g. the repo was renamed/removed from Artifactory after jf installed it. That's a genuine
	// "not found" case, not the "installed natively, unmanaged by jf" case.
	dir := pluginDir(t, `{"name":"web","version":"1.0.0"}`)
	notFound := fmt.Errorf("plugin '%s' %w '%s'", "web", plugincommon.ErrPluginNotFoundInRepo, "repo")
	target := plugincommon.AgentTarget{Agent: plugincommon.AgentSpec{Name: "claude"}, DestinationDir: dir}

	got := diagnoseNotFoundError(notFound, "web", "repo", []plugincommon.AgentTarget{target})
	assert.Same(t, notFound, got)
}

func TestDiagnoseNotFoundError_ExcludesAgentsWithoutNativeRegistry(t *testing.T) {
	restore := isRegisteredWithNativeAgent
	isRegisteredWithNativeAgent = func(agentName, _, _ string) (bool, error) {
		switch agentName {
		case "claude":
			return true, nil // actually installed
		case "codex":
			return false, nil // confirmed not installed
		default:
			return true, nil // mirrors the real IsRegisteredWithNativeAgent default for agents with no registry (e.g. cursor)
		}
	}
	t.Cleanup(func() { isRegisteredWithNativeAgent = restore })

	notFound := fmt.Errorf("plugin '%s' %w '%s'", "security-guidance", plugincommon.ErrPluginNotFoundInRepo, "claude-plugins-official")
	targets := []plugincommon.AgentTarget{
		{Agent: plugincommon.AgentSpec{Name: "claude"}, DestinationDir: filepath.Join(t.TempDir(), "claude-missing")},
		{Agent: plugincommon.AgentSpec{Name: "codex"}, DestinationDir: filepath.Join(t.TempDir(), "codex-missing")},
		{Agent: plugincommon.AgentSpec{Name: "cursor"}, DestinationDir: filepath.Join(t.TempDir(), "cursor-missing")},
	}

	got := diagnoseNotFoundError(notFound, "security-guidance", "claude-plugins-official", targets)
	require.Error(t, got)
	assert.Contains(t, got.Error(), "installed for claude")
	assert.NotContains(t, got.Error(), "codex", "codex was confirmed not registered")
	assert.NotContains(t, got.Error(), "cursor",
		"cursor has no native registry — IsRegisteredWithNativeAgent's unconditional true for it must not be read as a confirmed native install")
}

func TestResolveUpdateTargets_InjectsRepoKeyForClaude(t *testing.T) {
	globalBase := t.TempDir()
	opts := update{
		repoKey: "my-repo",
		flags: agentcommon.InstallFlagsResult{
			Specs:    []plugincommon.AgentSpec{{Name: "claude", Config: agentcommon.AgentConfig{GlobalDir: globalBase}}},
			IsGlobal: true,
		},
	}

	targets, err := resolveUpdateTargets(opts, "web")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(globalBase, "my-repo", "web"), targets[0].DestinationDir,
		"update must resolve to the same <GlobalDir>/<repoKey>/<slug> path install.go writes to")
}

func TestResolveUpdateTargets_CursorUnaffectedByRepoKey(t *testing.T) {
	globalBase := t.TempDir()
	opts := update{
		repoKey: "my-repo",
		flags: agentcommon.InstallFlagsResult{
			Specs:    []plugincommon.AgentSpec{{Name: "cursor", Config: agentcommon.AgentConfig{GlobalDir: globalBase}}},
			IsGlobal: true,
		},
	}

	targets, err := resolveUpdateTargets(opts, "web")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(globalBase, "web"), targets[0].DestinationDir)
}

func TestRunUpdate_AllRejectsSlugFlag(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "claude", "slug": "web"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --slug")
}

func TestRunUpdate_AllRejectsPositionalArg(t *testing.T) {
	ctx := newUpdateContext(t, []string{"web"}, map[string]string{"harness": "claude"}, map[string]bool{"all": true})
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected positional argument")
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

	// cursor: flat layout, no repo subdirectory.
	writeJFManagedPlugin(t, filepath.Join(installRoot, "shared"), "shared", "repo-x", "1.0.0")
	// claude: repo-keyed layout — nested one level under the repo name.
	writeJFManagedPlugin(t, filepath.Join(installRoot, "repo-x", "shared"), "shared", "repo-x", "1.0.0")

	flags := agentcommon.InstallFlagsResult{
		Specs: []plugincommon.AgentSpec{
			{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}},
			{Name: "claude", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}},
		},
		ProjectDirAbs: projectRoot,
	}

	discovered, err := discoverInstalledPluginTargets(flags, "")
	require.NoError(t, err)
	require.Len(t, discovered, 1)
	assert.Equal(t, "shared", discovered[0].slug)
	assert.Equal(t, "repo-x", discovered[0].repo)
	assert.Len(t, discovered[0].targets, 2, "both cursor's flat install and claude's repo-keyed install of the same slug@repo must merge into one entry")
}

func TestDiscoverInstalledPluginTargets_ScansEveryRepoSubdirectoryForClaudeAndCodex(t *testing.T) {
	// No --repo given: both repo subdirectories must be discovered without needing to
	// resolve a single repo up front.
	claudeGlobal := t.TempDir()
	claudeRepoA := filepath.Join(claudeGlobal, "repo-a", "alpha")
	claudeRepoB := filepath.Join(claudeGlobal, "repo-b", "beta")
	writeJFManagedPlugin(t, claudeRepoA, "alpha", "repo-a", "1.0.0")
	writeJFManagedPlugin(t, claudeRepoB, "beta", "repo-b", "2.0.0")

	codexGlobal := t.TempDir()
	codexRepoA := filepath.Join(codexGlobal, "repo-a", "alpha")
	writeJFManagedPlugin(t, codexRepoA, "alpha", "repo-a", "1.0.0")

	flags := agentcommon.InstallFlagsResult{
		Specs: []plugincommon.AgentSpec{
			{Name: "claude", Config: agentcommon.AgentConfig{GlobalDir: claudeGlobal}},
			{Name: "codex", Config: agentcommon.AgentConfig{GlobalDir: codexGlobal}},
		},
		IsGlobal: true,
	}

	discovered, err := discoverInstalledPluginTargets(flags, "")
	require.NoError(t, err)
	require.Len(t, discovered, 2, "alpha@repo-a (claude+codex) and beta@repo-b (claude only) must both be found")

	byKey := make(map[string]discoveredPlugin)
	for _, dp := range discovered {
		byKey[dp.slug+"@"+dp.repo] = dp
	}
	require.Contains(t, byKey, "alpha@repo-a")
	assert.Len(t, byKey["alpha@repo-a"].targets, 2)
	assert.Equal(t, claudeRepoA, byKey["alpha@repo-a"].targets[0].DestinationDir)
	assert.Equal(t, codexRepoA, byKey["alpha@repo-a"].targets[1].DestinationDir)

	require.Contains(t, byKey, "beta@repo-b")
	assert.Len(t, byKey["beta@repo-b"].targets, 1)
	assert.Equal(t, claudeRepoB, byKey["beta@repo-b"].targets[0].DestinationDir)
}

func TestDiscoverInstalledPluginTargets_RepoFilterExcludesMismatch(t *testing.T) {
	globalDir := t.TempDir()
	installed := filepath.Join(globalDir, "my-repo", "web")
	writeJFManagedPlugin(t, installed, "web", "my-repo", "1.0.0")

	flags := agentcommon.InstallFlagsResult{
		Specs:    []plugincommon.AgentSpec{{Name: "claude", Config: agentcommon.AgentConfig{GlobalDir: globalDir}}},
		IsGlobal: true,
	}

	filtered, err := discoverInstalledPluginTargets(flags, "wrong-repo")
	require.NoError(t, err)
	assert.Empty(t, filtered, "--all --repo <other> must exclude a plugin recorded under a different repo")

	unfiltered, err := discoverInstalledPluginTargets(flags, "")
	require.NoError(t, err)
	require.Len(t, unfiltered, 1, "without --repo, the plugin must still be found by scanning its repo subdirectory")
	assert.Equal(t, "my-repo", unfiltered[0].repo)
}

func TestDiscoverInstalledPluginTargets_SkipsDirectoriesWithoutPluginInfoManifest(t *testing.T) {
	projectRoot := t.TempDir()
	pluginsDir := "plugins"
	installRoot := filepath.Join(projectRoot, pluginsDir)
	legacyDir := filepath.Join(installRoot, "legacy")
	require.NoError(t, os.MkdirAll(legacyDir, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "plugin.json"), []byte(`{"name":"legacy","version":"0.9.0"}`), agentcommon.DefaultFileMode))
	// No .jfrog/plugin-info.json written: the plugin's own bundled manifest has no
	// repo field, so jf has no reliable repo to check for updates and must skip it,
	// not silently adopt it.

	flags := agentcommon.InstallFlagsResult{
		Specs:         []plugincommon.AgentSpec{{Name: "cursor", Config: agentcommon.AgentConfig{ProjectDir: pluginsDir}}},
		ProjectDirAbs: projectRoot,
	}

	discovered, err := discoverInstalledPluginTargets(flags, "")
	require.NoError(t, err)
	assert.Empty(t, discovered)
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

func TestFinalizeUpdateAll_PartialFailure(t *testing.T) {
	combined := []agentcommon.UpdateAllSummaryRow{
		{Agent: "cursor", Name: "good", Status: agentcommon.SummaryStatusOK},
		{Agent: "cursor", Name: "bad", Status: agentcommon.SummaryStatusFailed},
	}
	outcome := updateAllOutcome{anyOK: true, anyFailed: true}
	err := finalizeUpdateAll(combined, outcome, "table")
	require.Error(t, err, "one failed target among others must not be silently swallowed as success")
	assert.Contains(t, err.Error(), "failed for one or more targets")
}

func TestApplyUpdateAllForSlugs_ContinuesOnResolveError(t *testing.T) {
	oldResolve := resolveLatestPluginVersion
	defer func() {
		resolveLatestPluginVersion = oldResolve
	}()

	resolveLatestPluginVersion = func(_ *config.ServerDetails, _, slug string) (string, error) {
		if slug == "missing" {
			return "", errors.New("plugin 'missing' has no versions in repository 'repo'")
		}
		return "2.0.0", nil
	}

	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "cursor"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: "/tmp/missing",
	}
	opts := update{serverDetails: &config.ServerDetails{}}
	combined, outcome := applyUpdateAllForSlugs(opts, []discoveredPlugin{
		{slug: "missing", repo: "repo", targets: []plugincommon.AgentTarget{target}},
	})

	require.Len(t, combined, 1)
	assert.Equal(t, "missing", combined[0].Name)
	assert.Equal(t, agentcommon.SummaryStatusFailed, combined[0].Status)
	assert.Contains(t, combined[0].Detail, "no versions in repository")
	assert.Empty(t, combined[0].Version)
	assert.True(t, outcome.anyFailed)
	assert.False(t, outcome.anyOK)
	assert.Equal(t, 1, outcome.updatedSlugCount)
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
	updateSlugAcrossTargetsFn = func(opts update, slug, targetVersion string, targets []plugincommon.AgentTarget) ([]agentcommon.SummaryRow, error) {
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
	opts := update{serverDetails: &config.ServerDetails{}}
	combined, outcome := applyUpdateAllForSlugs(opts, []discoveredPlugin{
		{slug: "bad", repo: "repo", targets: []plugincommon.AgentTarget{target}},
		{slug: "good", repo: "repo", targets: []plugincommon.AgentTarget{{Agent: plugincommon.AgentSpec{Name: "cursor"}, Scope: plugincommon.ScopeProject, DestinationDir: "/tmp/good"}}},
	})

	require.Equal(t, 2, len(combined))
	assert.Equal(t, agentcommon.SummaryStatusFailed, combined[0].Status)
	assert.Equal(t, agentcommon.SummaryStatusOK, combined[1].Status)
	assert.True(t, outcome.anyOK)
	assert.True(t, outcome.anyFailed)
	assert.Equal(t, 2, outcome.updatedSlugCount)
}

func TestApplyUpdateAllForSlugs_UsesEachPluginsOwnRepo(t *testing.T) {
	oldResolve := resolveLatestPluginVersion
	oldUpdate := updateSlugAcrossTargetsFn
	defer func() {
		resolveLatestPluginVersion = oldResolve
		updateSlugAcrossTargetsFn = oldUpdate
	}()

	var resolvedRepos []string
	resolveLatestPluginVersion = func(_ *config.ServerDetails, repo, _ string) (string, error) {
		resolvedRepos = append(resolvedRepos, repo)
		return "2.0.0", nil
	}
	var updateRepos []string
	updateSlugAcrossTargetsFn = func(opts update, slug, targetVersion string, targets []plugincommon.AgentTarget) ([]agentcommon.SummaryRow, error) {
		updateRepos = append(updateRepos, opts.repoKey)
		return []agentcommon.SummaryRow{{Status: agentcommon.SummaryStatusOK}}, nil
	}

	// opts.repoKey is deliberately something neither plugin uses, to prove per-plugin
	// repo (not a single opts-wide repo) drives both resolution and the update call.
	opts := update{serverDetails: &config.ServerDetails{}, repoKey: "should-not-be-used"}
	discovered := []discoveredPlugin{
		{slug: "a", repo: "repo-1", targets: []plugincommon.AgentTarget{{Agent: plugincommon.AgentSpec{Name: "claude"}, DestinationDir: "/tmp/a"}}},
		{slug: "b", repo: "repo-2", targets: []plugincommon.AgentTarget{{Agent: plugincommon.AgentSpec{Name: "codex"}, DestinationDir: "/tmp/b"}}},
	}

	_, _ = applyUpdateAllForSlugs(opts, discovered)

	assert.Equal(t, []string{"repo-1", "repo-2"}, resolvedRepos)
	assert.Equal(t, []string{"repo-1", "repo-2"}, updateRepos)
}

func TestCreatePluginBackupForUpdate_MissingInstallDir(t *testing.T) {
	target := plugincommon.AgentTarget{
		Agent:          plugincommon.AgentSpec{Name: "cursor"},
		Scope:          plugincommon.ScopeProject,
		DestinationDir: filepath.Join(t.TempDir(), "missing"),
	}
	_, err := createPluginBackupForUpdate(target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "move current plugin aside")
}

func TestRestorePluginFromBackup_RestoresPreviousInstall(t *testing.T) {
	parent := t.TempDir()
	live := filepath.Join(parent, "web")
	backup := filepath.Join(parent, ".plugin-backup", "web-backup-test")

	require.NoError(t, os.MkdirAll(live, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(live, "plugin.json"), []byte(`{"name":"web","version":"1.0.0"}`), agentcommon.DefaultFileMode))
	require.NoError(t, os.MkdirAll(backup, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(backup, "plugin.json"), []byte(`{"name":"web","version":"0.9.0"}`), agentcommon.DefaultFileMode))

	require.NoError(t, os.MkdirAll(filepath.Join(live, "failed-copy"), agentcommon.InstallDirMode))

	target := plugincommon.AgentTarget{DestinationDir: live}
	require.NoError(t, restorePluginFromBackup(target, backup))

	data, err := os.ReadFile(filepath.Join(live, "plugin.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "0.9.0")
	_, err = os.Stat(backup)
	require.True(t, os.IsNotExist(err))
}

func TestConfirmUpdateAll_SkipsWhenDryRun(t *testing.T) {
	require.NoError(t, confirmUpdateAll(update{dryRun: true}))
}

func TestConfirmUpdateAll_SkipsWhenQuiet(t *testing.T) {
	require.NoError(t, confirmUpdateAll(update{quiet: true}))
}

func TestConfirmUpdateAll_SkipsWhenNonInteractive(t *testing.T) {
	oldCheck := isNonInteractive
	defer func() { isNonInteractive = oldCheck }()
	isNonInteractive = func() bool { return true }

	require.NoError(t, confirmUpdateAll(update{}))
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

func TestConfirmUpdateAll_ContinuesWhenUserAccepts(t *testing.T) {
	oldAsk := askYesNo
	oldCheck := isNonInteractive
	defer func() {
		askYesNo = oldAsk
		isNonInteractive = oldCheck
	}()
	isNonInteractive = func() bool { return false }
	askYesNo = func(_ string, _ bool) bool { return true }

	require.NoError(t, confirmUpdateAll(update{}))
}

func TestRunUpdate_RequiresSlugWithoutAll(t *testing.T) {
	ctx := newUpdateContext(t, nil, map[string]string{"harness": "claude"}, nil)
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestRunUpdate_RejectsPositionalSlug(t *testing.T) {
	ctx := newUpdateContext(t, []string{"web"}, map[string]string{"harness": "claude"}, nil)
	err := RunUpdate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use --slug")
}

func TestValidateUpdateScope_RejectsClaudeProjectScope(t *testing.T) {
	flags := agentcommon.InstallFlagsResult{
		Specs:         []plugincommon.AgentSpec{{Name: "claude"}},
		ProjectDirAbs: "/some/project",
		IsGlobal:      false,
	}
	err := validateUpdateScope(flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude does not support project-scoped plugin updates")
}

func TestValidateUpdateScope_AllowsGlobalScope(t *testing.T) {
	flags := agentcommon.InstallFlagsResult{
		Specs:    []plugincommon.AgentSpec{{Name: "claude"}},
		IsGlobal: true,
	}
	require.NoError(t, validateUpdateScope(flags))
}

func TestValidateUpdateScope_AllowsPathMode(t *testing.T) {
	flags := agentcommon.InstallFlagsResult{
		AbsoluteInstallBaseDir: "/some/path",
		IsGlobal:               false,
	}
	require.NoError(t, validateUpdateScope(flags), "--path mode is not project scope and must not be rejected")
}

func TestValidateUpdateScope_AllowsCustomAgentProjectScope(t *testing.T) {
	flags := agentcommon.InstallFlagsResult{
		Specs:    []plugincommon.AgentSpec{{Name: "my-custom-agent"}},
		IsGlobal: false,
	}
	require.NoError(t, validateUpdateScope(flags))
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
	require.NoError(t, os.MkdirAll(dir, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), agentcommon.DefaultFileMode))
	return dir
}

// writeJFManagedPlugin creates dir with both a plugin.json (so DiscoverInstalledPluginSlugs
// recognizes it as a plugin directory) and a .jfrog/plugin-info.json recording repo/version
// (so discoverInstalledPluginTargets recognizes it as jf-managed and knows which repo to
// check for updates).
func writeJFManagedPlugin(t *testing.T, dir, slug, repo, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, agentcommon.InstallDirMode))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"),
		fmt.Appendf(nil, `{"name":%q,"version":%q}`, slug, version), agentcommon.DefaultFileMode))
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, plugincommon.PluginInfoManifestFile, plugincommon.PluginInfoManifest{
		Repo: repo, Slug: slug, InstalledVersion: version, Scope: "global", Agent: "test",
	}))
}
