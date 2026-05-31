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
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// pluginBackupDirName is the directory under the plugins parent where update backups are stored.
const pluginBackupDirName = ".plugin-backup"

// resolveLatestPluginVersion is swappable in tests.
var resolveLatestPluginVersion = plugincommon.ResolveLatestPluginVersion

type preUpdate struct {
	agentTarget            plugincommon.AgentTarget
	installedVersion       string
	alreadyAtTargetVersion bool
	failureReason          string
}

// RunUpdate is the CLI action for `jf agent plugins update`.
func RunUpdate(c *components.Context) error {
	all := c.GetBoolFlagValue("all")
	hasSlugArg := c.GetNumberOfArgs() >= 1
	if !all && !hasSlugArg {
		return fmt.Errorf("usage: jf agent plugins update <slug> (--harness <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>] [--dry-run] [--force] [--format <table|json>]\n       jf agent plugins update --all --harness <name[,name...]> [--global] [--project-dir <dir>] [--repo <repo>] [--dry-run] [--force] [--format <table|json>]")
	}
	if all {
		if hasSlugArg {
			return fmt.Errorf("--all cannot be combined with a slug argument; it updates every installed plugin for the given --harness list")
		}
		if strings.TrimSpace(c.GetStringFlagValue("version")) != "" {
			return fmt.Errorf("--all cannot be combined with --version; it always updates to the latest version")
		}
		if strings.TrimSpace(c.GetStringFlagValue("path")) != "" {
			return fmt.Errorf("--all cannot be combined with --path; --path targets a single install directory")
		}
	}

	flags, err := plugincommon.ValidateInstallFlags(c)
	if err != nil {
		return err
	}
	if all && flags.AbsoluteInstallBaseDir != "" {
		return fmt.Errorf("--all requires --harness; --path is not supported")
	}
	if all && len(flags.Specs) == 0 {
		return fmt.Errorf("--all requires --harness <name[,name...]>")
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}
	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, plugincommon.RepoOptions())
	if err != nil {
		return err
	}

	dryRun := c.GetBoolFlagValue("dry-run")
	force := c.GetBoolFlagValue("force")
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	opts := updateOptions{
		serverDetails: serverDetails,
		repoKey:       repoKey,
		flags:         flags,
		dryRun:        dryRun,
		force:         force,
		format:        format,
		quiet:         quiet,
	}

	if all {
		return runUpdateAll(opts)
	}

	slug := c.GetArgumentAt(0)
	if err := plugincommon.ValidateSlug(slug); err != nil {
		return err
	}
	requestedVersion := strings.TrimSpace(c.GetStringFlagValue("version"))
	return runUpdateOne(opts, slug, requestedVersion)
}

type updateOptions struct {
	serverDetails *config.ServerDetails
	repoKey       string
	flags         agentcommon.InstallFlagsResult
	dryRun        bool
	force         bool
	format        string
	quiet         bool
}

// runUpdateOne updates a single slug across all resolved targets.
func runUpdateOne(opts updateOptions, slug, requestedVersion string) error {
	targets, err := plugincommon.ResolveAgentTargets(slug, opts.flags.AbsoluteInstallBaseDir, opts.flags.Specs, opts.flags.ProjectDirAbs, opts.flags.IsGlobal)
	if err != nil {
		return err
	}

	targetVersion, err := resolveTargetVersion(opts.serverDetails, opts.repoKey, slug, requestedVersion)
	if err != nil {
		return err
	}

	results, err := updateSlugAcrossTargets(opts, slug, targetVersion, targets)
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

// runUpdateAll enumerates every installed plugin under each --harness and updates each to its latest version.
func runUpdateAll(opts updateOptions) error {
	slugOrder, slugToTargets, err := discoverInstalledPluginTargets(opts.flags)
	if err != nil {
		return err
	}
	if len(slugOrder) == 0 {
		log.Info("No installed plugins found for the given --harness list; nothing to update.")
		return nil
	}

	combined, outcome, err := applyUpdateAllForSlugs(opts, slugOrder, slugToTargets)
	if err != nil {
		return err
	}
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
		slugs, err := agentcommon.DiscoverInstalledSlugs(installDir, plugincommon.PluginInfoManifestFile)
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
func applyUpdateAllForSlugs(
	opts updateOptions,
	slugOrder []string,
	slugToTargets map[string][]plugincommon.AgentTarget,
) ([]agentcommon.UpdateAllSummaryRow, updateAllOutcome, error) {
	combined := make([]agentcommon.UpdateAllSummaryRow, 0)
	var outcome updateAllOutcome
	for _, slug := range slugOrder {
		targetVersion, err := resolveLatestPluginVersion(opts.serverDetails, opts.repoKey, slug)
		if err != nil {
			if outcome.firstResolveErr == nil {
				outcome.firstResolveErr = err
			}
			log.Warn(fmt.Sprintf("Skipping plugin '%s': could not resolve latest version: %s", slug, err.Error()))
			continue
		}
		results, err := updateSlugAcrossTargets(opts, slug, targetVersion, slugToTargets[slug])
		if err != nil {
			return nil, updateAllOutcome{}, err
		}
		combined = agentcommon.AppendUpdateAllSummaryRows(combined, slug, targetVersion, results)
		outcome.updatedSlugCount++
		ok, failed := tallySummaryRows(results)
		outcome.anyOK = outcome.anyOK || ok
		outcome.anyFailed = outcome.anyFailed || failed
	}
	return combined, outcome, nil
}

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

func resolveTargetVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "latest" {
		if err := plugincommon.ValidateVersion(requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	return resolveLatestPluginVersion(serverDetails, repoKey, slug)
}

// updateSlugAcrossTargets fetches the slug once and runs the backup+copy loop per target.
// Returns the per-target summary rows. Targets that are not installed or already at the
// target version are reported without performing a download.
func updateSlugAcrossTargets(opts updateOptions, slug, targetVersion string, targets []plugincommon.AgentTarget) ([]agentcommon.SummaryRow, error) {
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
		_ = os.RemoveAll(tmpDir)
	}()

	cmd := install.NewInstallCommand().
		SetServerDetails(opts.serverDetails).
		SetRepoKey(opts.repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetQuiet(opts.quiet).
		SetProjectDir(opts.flags.ProjectDirAbs).
		SetGlobal(opts.flags.IsGlobal)

	unzipDir, err := cmd.FetchAndExtractTo(tmpDir)
	if err != nil {
		return nil, err
	}

	for _, preUpdateCheck := range updatable {
		results = append(results, updateOnePlugin(unzipDir, cmd, preUpdateCheck))
	}
	return results, nil
}

func preUpdateTargets(targets []plugincommon.AgentTarget, targetVersion string, force, quiet bool) []preUpdate {
	out := make([]preUpdate, 0, len(targets))
	for _, agentTarget := range targets {
		record := preUpdate{agentTarget: agentTarget}
		installedVersion, err := plugincommon.ReadInstalledPluginVersion(agentTarget.DestinationDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				record.failureReason = fmt.Sprintf("plugin not installed at %s; run 'jf agent plugins install' first", agentTarget.DestinationDir)
			} else {
				record.failureReason = err.Error()
			}
			if !quiet {
				log.Info(fmt.Sprintf("Skipping update for agent %s at %s: %s", agentTarget.Agent.Name, agentTarget.DestinationDir, record.failureReason))
			}
			out = append(out, record)
			continue
		}
		record.installedVersion = installedVersion
		if installedVersion == targetVersion && !force {
			record.alreadyAtTargetVersion = true
			if !quiet {
				log.Info(fmt.Sprintf("Skipping update for agent %s at %s: already at version %s (use --force to re-download)", agentTarget.Agent.Name, agentTarget.DestinationDir, targetVersion))
			}
		}
		out = append(out, record)
	}
	return out
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

// updateOnePlugin updates a single install target using the already-fetched tree in unzipDir.
func updateOnePlugin(unzipDir string, installCommand *install.InstallCommand, check preUpdate) agentcommon.SummaryRow {
	agentTarget := check.agentTarget
	backupPath, errRow, ok := movePluginAsideForUpdate(agentTarget)
	if !ok {
		return errRow
	}
	row := applyPluginUpdateCopy(unzipDir, installCommand, agentTarget, backupPath)
	if row.Status != agentcommon.SummaryStatusOK {
		return row
	}
	removePluginUpdateBackup(backupPath, filepath.Dir(agentTarget.DestinationDir))
	return summaryRowFor(agentTarget, agentcommon.SummaryStatusOK, agentcommon.SummaryDetailOKInstall)
}

// movePluginAsideForUpdate reserves a backup path and renames the live install directory aside.
func movePluginAsideForUpdate(agentTarget plugincommon.AgentTarget) (backupPath string, errRow agentcommon.SummaryRow, ok bool) {
	slugBase := filepath.Base(agentTarget.DestinationDir)
	parent := filepath.Dir(agentTarget.DestinationDir)
	backupPath, err := reserveUpdateBackupPath(parent, slugBase)
	if err != nil {
		return "", summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, err.Error()), false
	}
	if err := os.Rename(agentTarget.DestinationDir, backupPath); err != nil {
		return "", summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, fmt.Sprintf("could not move current plugin aside for update: %s", err.Error())), false
	}
	return backupPath, agentcommon.SummaryRow{}, true
}

// restorePluginFromBackup removes a failed new install and renames the backup back into place.
func restorePluginFromBackup(agentTarget plugincommon.AgentTarget, backupPath string) error {
	_ = os.RemoveAll(agentTarget.DestinationDir)
	return os.Rename(backupPath, agentTarget.DestinationDir)
}

// applyPluginUpdateCopy copies the extracted tree onto the target and restores the backup on failure.
func applyPluginUpdateCopy(
	unzipDir string,
	installCommand *install.InstallCommand,
	agentTarget plugincommon.AgentTarget,
	backupPath string,
) agentcommon.SummaryRow {
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
	_ = os.Remove(backupRoot)
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
	// #nosec G301 -- update backup dir under user plugin tree; permissive mode matches install copy behavior for tooling access.
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s directory: %w", pluginBackupDirName, err)
	}
	pattern := slug + "-backup-*"
	d, err := os.MkdirTemp(backupRoot, pattern)
	if err != nil {
		return "", fmt.Errorf("could not reserve update backup path: %w", err)
	}
	if err := os.Remove(d); err != nil {
		return "", fmt.Errorf("could not prepare update backup path: %w", err)
	}
	return d, nil
}
