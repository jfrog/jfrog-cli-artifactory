package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/install"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// askYesNo is swappable in tests.
var askYesNo = coreutils.AskYesNo

// isNonInteractive is swappable in tests (GitHub Actions sets CI=true).
var isNonInteractive = agentcommon.IsNonInteractive

const updateAllConfirmPrompt = "Update all discovered plugins under the given harness(es) to their latest version in the repository? " +
	"Each install folder name is used as the repository slug (same as update --slug). " +
	"Matching packages will be updated, including installs that were not made with JFrog CLI."

// pluginBackupDirName is the directory under the plugins parent where update backups are stored.
const pluginBackupDirName = ".plugin-backup"

// resolveLatestPluginVersion is swappable in tests.
var resolveLatestPluginVersion = plugincommon.ResolveLatestPluginVersion

// resolvePluginVersion is swappable in tests.
var resolvePluginVersion = plugincommon.ResolvePluginVersion

// updateSlugAcrossTargetsFn is swappable in tests.
var updateSlugAcrossTargetsFn = updateSlugAcrossTargets

type preUpdate struct {
	agentTarget            plugincommon.AgentTarget
	installedVersion       string
	alreadyAtTargetVersion bool
	failureReason          string
}

// RunUpdate is the CLI action for `jf agent plugins update`.
func RunUpdate(c *components.Context) error {
	all := c.GetBoolFlagValue("all")
	slugFlag := strings.TrimSpace(c.GetStringFlagValue("slug"))
	if !all && slugFlag == "" {
		if c.GetNumberOfArgs() > 0 {
			return fmt.Errorf("unexpected positional argument(s); use --slug to specify the plugin")
		}
		return fmt.Errorf("usage: jf agent plugins update --slug <slug> (--harness <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>] [--dry-run] [--force] [--format <table|json>]\n       jf agent plugins update --all --harness <name[,name...]> [--global] [--project-dir <dir>] [--repo <repo>] [--dry-run] [--force] [--format <table|json>]")
	}
	if all {
		if slugFlag != "" {
			return fmt.Errorf("--all cannot be combined with --slug; it updates every installed plugin for the given --harness list")
		}
		if c.GetNumberOfArgs() > 0 {
			return fmt.Errorf("unexpected positional argument(s); use --slug or --all")
		}
		if strings.TrimSpace(c.GetStringFlagValue("version")) != "" {
			return fmt.Errorf("--all cannot be combined with --version; it always updates to the latest version")
		}
		if strings.TrimSpace(c.GetStringFlagValue("path")) != "" {
			return fmt.Errorf("--all cannot be combined with --path; --path targets a single install directory")
		}
	}

	opts, err := newUpdate(c)
	if err != nil {
		return err
	}
	if all && opts.flags.AbsoluteInstallBaseDir != "" {
		return fmt.Errorf("--all requires --harness; --path is not supported")
	}
	if all && len(opts.flags.Specs) == 0 {
		return fmt.Errorf("--all requires --harness <name[,name...]>")
	}

	if all {
		if err := confirmUpdateAll(opts); err != nil {
			return err
		}
		return runUpdateAll(opts)
	}

	if c.GetNumberOfArgs() > 0 {
		return fmt.Errorf("unexpected positional argument(s); use --slug to specify the plugin")
	}
	if err := agentcommon.ValidateSlug(slugFlag); err != nil {
		return err
	}
	requestedVersion := strings.TrimSpace(c.GetStringFlagValue("version"))
	return runUpdateOnSlug(opts, slugFlag, requestedVersion)
}

type update struct {
	serverDetails *config.ServerDetails
	repoKey       string
	flags         agentcommon.InstallFlagsResult
	dryRun        bool
	force         bool
	format        string
	quiet         bool
}

func newUpdate(c *components.Context) (update, error) {
	flags, err := plugincommon.ValidateInstallFlags(c)
	if err != nil {
		return update{}, err
	}
	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return update{}, err
	}
	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, plugincommon.RepoOptions())
	if err != nil {
		return update{}, err
	}
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}
	return update{
		serverDetails: serverDetails,
		repoKey:       repoKey,
		flags:         flags,
		dryRun:        c.GetBoolFlagValue("dry-run"),
		force:         c.GetBoolFlagValue("force"),
		format:        format,
		quiet:         quiet,
	}, nil
}

// runUpdateOnSlug updates a single slug across all resolved targets.
func runUpdateOnSlug(opts update, slug, requestedVersion string) error {
	targets, err := agentcommon.ResolveAgentTargets(slug, opts.flags.AbsoluteInstallBaseDir, opts.flags.Specs, opts.flags.ProjectDirAbs, opts.flags.IsGlobal)
	if err != nil {
		return err
	}

	targetVersion, err := resolveTargetVersion(opts.serverDetails, opts.repoKey, slug, requestedVersion, opts.quiet)
	if err != nil {
		return err
	}

	results, err := updateSlugAcrossTargetsFn(opts, slug, targetVersion, targets)
	if err != nil {
		return err
	}
	if err := agentcommon.PrintInstallSummary("Plugin", slug, targetVersion, results, opts.format); err != nil {
		return err
	}
	return finalError(results)
}

// updateAllOutcome tracks aggregate success/failure for a --all run.
type updateAllOutcome struct {
	anyOK            bool
	anyFailed        bool
	firstResolveErr  error
	updatedSlugCount int
}

// confirmUpdateAll asks for interactive confirmation before update --all (skipped for --dry-run, --quiet, and CI).
func confirmUpdateAll(opts update) error {
	if opts.dryRun || opts.quiet || isNonInteractive() {
		return nil
	}
	if !askYesNo(updateAllConfirmPrompt, false) {
		return fmt.Errorf("update --all aborted by user")
	}
	return nil
}

// runUpdateAll enumerates every installed plugin under each --harness and updates each to its latest version.
func runUpdateAll(opts update) error {
	slugOrder, slugToTargets, err := discoverInstalledPluginTargets(opts.flags)
	if err != nil {
		return err
	}
	if len(slugOrder) == 0 {
		log.Info("No installed plugins found for the given --harness list; nothing to update.")
		return nil
	}

	combined, outcome := applyUpdateAllForSlugs(opts, slugOrder, slugToTargets)
	return finalizeUpdateAll(combined, outcome, opts.format)
}

// discoverInstalledPluginTargets maps each installed slug to its harness install targets.
func discoverInstalledPluginTargets(flags agentcommon.InstallFlagsResult) ([]string, map[string][]plugincommon.AgentTarget, error) {
	slugToTargets := make(map[string][]plugincommon.AgentTarget)
	slugOrder := make([]string, 0)
	scope := plugincommon.ScopeProject
	if flags.IsGlobal {
		scope = plugincommon.ScopeGlobal
	}
	for _, spec := range flags.Specs {
		installDir, err := agentcommon.ResolveAgentInstallDir(spec, flags.ProjectDirAbs, flags.IsGlobal)
		if err != nil {
			return nil, nil, err
		}
		slugs, err := plugincommon.DiscoverInstalledPluginSlugs(installDir)
		if err != nil {
			return nil, nil, err
		}
		for _, slug := range slugs {
			if _, seen := slugToTargets[slug]; !seen {
				slugOrder = append(slugOrder, slug)
			}
			slugToTargets[slug] = append(slugToTargets[slug], plugincommon.AgentTarget{
				Agent:          spec,
				Scope:          scope,
				DestinationDir: filepath.Join(installDir, slug),
			})
		}
	}
	return slugOrder, slugToTargets, nil
}

// applyUpdateAllForSlugs resolves latest version per slug, updates targets, and builds combined summary rows.
// Resolve and download failures for one slug are logged and recorded as failed rows; remaining slugs still run.
func applyUpdateAllForSlugs(opts update, slugOrder []string, slugToTargets map[string][]plugincommon.AgentTarget,
) ([]agentcommon.UpdateAllSummaryRow, updateAllOutcome) {
	combined := make([]agentcommon.UpdateAllSummaryRow, 0)
	var outcome updateAllOutcome
	for _, slug := range slugOrder {
		targetVersion, err := resolveLatestPluginVersion(opts.serverDetails, opts.repoKey, slug)
		if err != nil {
			if outcome.firstResolveErr == nil {
				outcome.firstResolveErr = err
			}
			log.Warn(fmt.Sprintf("Skipping plugin '%s': could not resolve latest version: %s", slug, err.Error()))
			results := failedRowsForTargets(slugToTargets[slug], err.Error())
			combined = agentcommon.AppendUpdateAllSummaryRows(combined, slug, "", results)
			outcome.updatedSlugCount++
			_, slugFailed := tallySummaryRows(results)
			outcome.anyFailed = outcome.anyFailed || slugFailed
			continue
		}
		results, err := updateSlugAcrossTargetsFn(opts, slug, targetVersion, slugToTargets[slug])
		if err != nil {
			log.Warn(fmt.Sprintf("Skipping plugin '%s': download failed: %s", slug, err.Error()))
			results = failedRowsForTargets(slugToTargets[slug], err.Error())
		}
		combined = agentcommon.AppendUpdateAllSummaryRows(combined, slug, targetVersion, results)
		outcome.updatedSlugCount++
		slugOK, slugFailed := tallySummaryRows(results)
		outcome.anyOK = outcome.anyOK || slugOK
		outcome.anyFailed = outcome.anyFailed || slugFailed
	}
	return combined, outcome
}

func failedRowsForTargets(targets []plugincommon.AgentTarget, detail string) []agentcommon.SummaryRow {
	rows := make([]agentcommon.SummaryRow, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, summaryRowFor(target, agentcommon.SummaryStatusFailed, detail))
	}
	return rows
}

// tallySummaryRows reports whether any per-target row succeeded (ok) or failed.
// Skipped rows are ignored. Used during update --all to aggregate exit conditions across slugs.
func tallySummaryRows(results []agentcommon.SummaryRow) (anyOK, anyFailed bool) {
	for _, row := range results {
		switch row.Status {
		case agentcommon.SummaryStatusOK:
			anyOK = true
		case agentcommon.SummaryStatusFailed:
			anyFailed = true
		}
	}
	return anyOK, anyFailed
}

func finalizeUpdateAll(combined []agentcommon.UpdateAllSummaryRow, outcome updateAllOutcome, format string) error {
	if err := agentcommon.PrintUpdateAllSummary("Plugin", combined, format); err != nil {
		return err
	}
	if !outcome.anyOK && outcome.anyFailed {
		return fmt.Errorf("update failed for all targets (see summary above)")
	}
	if !outcome.anyOK && outcome.updatedSlugCount == 0 && outcome.firstResolveErr != nil {
		return outcome.firstResolveErr
	}
	return nil
}

func resolveTargetVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	return resolvePluginVersion(serverDetails, repoKey, slug, requested, quiet)
}

// updateSlugAcrossTargets fetches the slug once and runs the backup+copy loop per target.
// Returns the per-target summary rows. Targets that are not installed or already at the
// target version are reported without performing a download.
func updateSlugAcrossTargets(opts update, slug, targetVersion string, targets []plugincommon.AgentTarget) ([]agentcommon.SummaryRow, error) {
	checks := preUpdateTargets(targets, targetVersion, opts.force, opts.quiet)
	results, updatable := initialResultsAndUpdatable(checks, targetVersion)

	if opts.dryRun {
		logDryRun(slug, targetVersion, checks)
		return results, nil
	}
	if len(updatable) == 0 {
		return results, nil
	}

	tmpDir, err := os.MkdirTemp("", "plugin-update-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		// Best-effort teardown of per-slug temp dir after copies finish or fail.
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			log.Warn(fmt.Sprintf("Could not remove plugin update temp dir %s: %s", tmpDir, removeErr.Error()))
		}
	}()

	installCmd := install.NewInstallCommand().
		SetServerDetails(opts.serverDetails).
		SetRepoKey(opts.repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetQuiet(opts.quiet).
		SetProjectDir(opts.flags.ProjectDirAbs).
		SetGlobal(opts.flags.IsGlobal)

	unzipDir, err := installCmd.FetchAndExtractTo(tmpDir)
	if err != nil {
		return nil, err
	}

	for _, preUpdateCheck := range updatable {
		results = append(results, updatePlugin(unzipDir, installCmd, preUpdateCheck))
	}
	return results, nil
}

func preUpdateTargets(targets []plugincommon.AgentTarget, targetVersion string, force, quiet bool) []preUpdate {
	checks := make([]preUpdate, 0, len(targets))
	for _, agentTarget := range targets {
		preUpdateCheck := preUpdate{agentTarget: agentTarget}
		installedVersion, err := plugincommon.ReadInstalledPluginVersion(agentTarget.DestinationDir)
		if err != nil {
			slug := filepath.Base(agentTarget.DestinationDir)
			log.Warn(fmt.Sprintf("Skipping plugin '%s': %s", slug, err.Error()))
			if errors.Is(err, fs.ErrNotExist) {
				preUpdateCheck.failureReason = fmt.Sprintf("plugin not installed at %s; run 'jf agent plugins install' first", agentTarget.DestinationDir)
			} else {
				preUpdateCheck.failureReason = err.Error()
			}
			checks = append(checks, preUpdateCheck)
			continue
		}
		preUpdateCheck.installedVersion = installedVersion
		if installedVersion == targetVersion && !force {
			preUpdateCheck.alreadyAtTargetVersion = true
			if !quiet {
				log.Info(fmt.Sprintf("Skipping update for agent %s at %s: already at version %s (use --force to re-download)", agentTarget.Agent.Name, agentTarget.DestinationDir, targetVersion))
			}
		}
		checks = append(checks, preUpdateCheck)
	}
	return checks
}

func initialResultsAndUpdatable(checks []preUpdate, targetVersion string) ([]agentcommon.SummaryRow, []preUpdate) {
	results := make([]agentcommon.SummaryRow, 0, len(checks))
	updatable := make([]preUpdate, 0, len(checks))
	for _, preUpdateCheck := range checks {
		switch {
		case preUpdateCheck.failureReason != "":
			results = append(results, summaryRowFor(preUpdateCheck.agentTarget, agentcommon.SummaryStatusFailed, preUpdateCheck.failureReason))
		case preUpdateCheck.alreadyAtTargetVersion:
			results = append(results, summaryRowFor(preUpdateCheck.agentTarget, agentcommon.SummaryStatusSkipped, fmt.Sprintf("version already %s; use --force to reinstall", targetVersion)))
		default:
			updatable = append(updatable, preUpdateCheck)
		}
	}
	return results, updatable
}

func summaryRowFor(agentTarget plugincommon.AgentTarget, status, detail string) agentcommon.SummaryRow {
	return agentcommon.SummaryRow{
		Agent:  agentTarget.Agent.Name,
		Scope:  string(agentTarget.Scope),
		Path:   agentTarget.DestinationDir,
		Status: status,
		Detail: detail,
	}
}

func logDryRun(slug, targetVersion string, checks []preUpdate) {
	for _, preUpdateCheck := range checks {
		switch {
		case preUpdateCheck.failureReason != "":
			log.Info(fmt.Sprintf("[dry-run] Would skip %s at %s: %s", slug, preUpdateCheck.agentTarget.DestinationDir, preUpdateCheck.failureReason))
		case preUpdateCheck.alreadyAtTargetVersion:
			log.Info(fmt.Sprintf("[dry-run] Plugin '%s' already at v%s at %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		case preUpdateCheck.installedVersion == "":
			log.Info(fmt.Sprintf("[dry-run] Would install plugin '%s' v%s to %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		default:
			log.Info(fmt.Sprintf("[dry-run] Would update plugin '%s' from v%s -> v%s at %s", slug, preUpdateCheck.installedVersion, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		}
	}
}

// updatePlugin updates a single install target using the already-fetched tree in unzipDir.
// On success the backup is deleted; on copy failure applyPluginUpdateCopy restores the backup first.
func updatePlugin(unzipDir string, installCommand *install.InstallCommand, check preUpdate) agentcommon.SummaryRow {
	agentTarget := check.agentTarget
	backupPath, err := createPluginBackupForUpdate(agentTarget)
	if err != nil {
		return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, err.Error())
	}
	row := applyPluginUpdateCopy(unzipDir, installCommand, agentTarget, backupPath)
	if row.Status != agentcommon.SummaryStatusOK {
		return row
	}
	removePluginUpdateBackup(backupPath, filepath.Dir(agentTarget.DestinationDir))
	return summaryRowFor(agentTarget, agentcommon.SummaryStatusOK, agentcommon.SummaryDetailOKInstall)
}

// createPluginBackupForUpdate reserves a backup path and renames the live install directory aside.
func createPluginBackupForUpdate(agentTarget plugincommon.AgentTarget) (string, error) {
	slugBase := filepath.Base(agentTarget.DestinationDir)
	parent := filepath.Dir(agentTarget.DestinationDir)
	backupPath, err := reserveUpdateBackupPath(parent, slugBase)
	if err != nil {
		return "", err
	}
	if err := os.Rename(agentTarget.DestinationDir, backupPath); err != nil {
		return "", fmt.Errorf("could not move current plugin aside for update: %w", err)
	}
	return backupPath, nil
}

// restorePluginFromBackup removes a failed new install and renames the backup back into place.
func restorePluginFromBackup(agentTarget plugincommon.AgentTarget, backupPath string) error {
	if err := os.RemoveAll(agentTarget.DestinationDir); err != nil {
		log.Warn(fmt.Sprintf("Could not remove failed plugin install at %s before restore: %s", agentTarget.DestinationDir, err.Error()))
	}
	if err := os.Rename(backupPath, agentTarget.DestinationDir); err != nil {
		return fmt.Errorf("could not restore previous plugin install from %s: %w", backupPath, err)
	}
	return nil
}

// applyPluginUpdateCopy installs the extracted plugin tree at agentTarget.
// The live install must already have been moved aside to backupPath (createPluginBackupForUpdate).
// If the copy fails or returns a non-ok summary row, restorePluginFromBackup removes any partial
// new install and renames the backup back to DestinationDir so the previous version remains in place.
func applyPluginUpdateCopy(unzipDir string, installCommand *install.InstallCommand, agentTarget plugincommon.AgentTarget, backupPath string) agentcommon.SummaryRow {
	rows := installCommand.CopyExtractedToTargets(unzipDir, []plugincommon.AgentTarget{agentTarget})
	if len(rows) != 1 {
		if restoreErr := restorePluginFromBackup(agentTarget, backupPath); restoreErr != nil {
			return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, fmt.Sprintf("internal error: unexpected copy result count; restore failed: %s", restoreErr.Error()))
		}
		return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, "internal error: unexpected copy result count")
	}
	row := rows[0]
	if row.Status != agentcommon.SummaryStatusOK {
		if restoreErr := restorePluginFromBackup(agentTarget, backupPath); restoreErr != nil {
			row.Detail = fmt.Sprintf("%s; could not restore previous install: %s", row.Detail, restoreErr.Error())
		}
		return row
	}
	return row
}

// removePluginUpdateBackup deletes the backup tree after a successful update.
func removePluginUpdateBackup(backupPath, parent string) {
	if err := os.RemoveAll(backupPath); err != nil {
		log.Warn(fmt.Sprintf("Update succeeded but previous copy at %s could not be deleted: %s", backupPath, err.Error()))
		return
	}
	backupRoot := filepath.Join(parent, pluginBackupDirName)
	if err := os.Remove(backupRoot); err != nil && !os.IsNotExist(err) {
		log.Warn(fmt.Sprintf("Could not remove empty %s directory at %s: %s", pluginBackupDirName, backupRoot, err.Error()))
	}
}

func finalError(results []agentcommon.SummaryRow) error {
	if len(results) == 0 {
		return nil
	}
	for _, result := range results {
		if result.Status != agentcommon.SummaryStatusFailed {
			return nil
		}
	}
	return fmt.Errorf("update failed for all targets (see summary above)")
}

func reserveUpdateBackupPath(installBase, slug string) (string, error) {
	backupRoot := filepath.Join(installBase, pluginBackupDirName)
	if err := os.MkdirAll(backupRoot, agentcommon.InstallDirMode); err != nil {
		return "", fmt.Errorf("could not create %s directory: %w", pluginBackupDirName, err)
	}
	pattern := slug + "-backup-*"
	reservedBackupPath, err := os.MkdirTemp(backupRoot, pattern)
	if err != nil {
		return "", fmt.Errorf("could not reserve update backup path: %w", err)
	}
	if err := os.Remove(reservedBackupPath); err != nil {
		return "", fmt.Errorf("could not prepare update backup path: %w", err)
	}
	return reservedBackupPath, nil
}
