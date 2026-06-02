package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// askYesNo is swappable in tests.
var askYesNo = coreutils.AskYesNo

// isNonInteractive is swappable in tests (GitHub Actions sets CI=true).
var isNonInteractive = agentcommon.IsNonInteractive

const updateAllConfirmPrompt = "Update all discovered skills under the given harness(es) to their latest version in the repository? " +
	"Matching packages will be updated, including installs that were not made with JFrog CLI."

// skillBackupDirName is the directory under the skills parent where update backups are stored.
const skillBackupDirName = ".skill-backup"

// resolveLatestSkillVersion is swappable in tests.
var resolveLatestSkillVersion = common.ResolveLatestSkillVersion

// updateSlugAcrossTargetsFn is swappable in tests.
var updateSlugAcrossTargetsFn = updateSlugAcrossTargets

type preUpdate struct {
	agentTarget            common.AgentTarget
	installedVersion       string
	alreadyAtTargetVersion bool
	failureReason          string
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

// RunUpdate is the CLI action for `jf agent skills update`.
func RunUpdate(c *components.Context) error {
	all := c.GetBoolFlagValue("all")
	slugFlag := strings.TrimSpace(c.GetStringFlagValue("slug"))
	if err := validateUpdateArgs(c, all, slugFlag); err != nil {
		return err
	}

	updateParams, err := newUpdate(c)
	if err != nil {
		return err
	}
	if all {
		return runUpdateAllMode(updateParams)
	}
	return runSingleSlugUpdate(c, updateParams, slugFlag)
}

// validateUpdateArgs checks --slug vs --all and rejects incompatible flag combinations.
func validateUpdateArgs(c *components.Context, all bool, slugFlag string) error {
	if !all && slugFlag == "" {
		if c.GetNumberOfArgs() > 0 {
			return fmt.Errorf("unexpected positional argument(s); use --slug to specify the skill")
		}
		return fmt.Errorf("usage: jf agent skills update --slug <slug> (--harness <name[,name...]> [--global] [--project-dir <dir>] | --path <dir>) [--repo <repo>] [--version <ver>] [--dry-run] [--force] [--format <table|json>]\n       jf agent skills update --all --harness <name[,name...]> [--global] [--project-dir <dir>] [--repo <repo>] [--dry-run] [--force] [--format <table|json>]")
	}
	if !all {
		return nil
	}
	if slugFlag != "" {
		return fmt.Errorf("--all cannot be combined with --slug; it updates every installed skill for the given --harness list")
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
	return nil
}

// validateUpdateAllTargets ensures --all was given with --harness and not --path.
func validateUpdateAllTargets(flags agentcommon.InstallFlagsResult) error {
	if flags.AbsoluteInstallBaseDir != "" {
		return fmt.Errorf("--all requires --harness; --path is not supported")
	}
	if len(flags.Specs) == 0 {
		return fmt.Errorf("--all requires --harness <name[,name...]>")
	}
	return nil
}

// runUpdateAllMode confirms with the user (when interactive) and runs update --all.
func runUpdateAllMode(updateParams update) error {
	if err := validateUpdateAllTargets(updateParams.flags); err != nil {
		return err
	}
	if err := confirmUpdateAll(updateParams); err != nil {
		return err
	}
	return runUpdateAll(updateParams)
}

// runSingleSlugUpdate validates --slug and updates one skill across resolved targets.
func runSingleSlugUpdate(c *components.Context, updateParams update, slugFlag string) error {
	if c.GetNumberOfArgs() > 0 {
		return fmt.Errorf("unexpected positional argument(s); use --slug to specify the skill")
	}
	if err := publish.ValidateSlug(slugFlag); err != nil {
		return err
	}
	requestedVersion := strings.TrimSpace(c.GetStringFlagValue("version"))
	return runUpdateOnSlug(updateParams, slugFlag, requestedVersion)
}

// newUpdate parses flags, server config, and repo into the shared update state.
func newUpdate(c *components.Context) (update, error) {
	flags, err := common.ValidateInstallFlags(c)
	if err != nil {
		return update{}, err
	}
	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return update{}, err
	}
	quiet := agentcommon.IsQuiet(c)
	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet, common.RepoOptions())
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

// runUpdateOnSlug resolves the target version, updates all harness targets for one slug, and prints the summary.
func runUpdateOnSlug(updateParams update, slug, requestedVersion string) error {
	targets, err := common.ResolveAgentTargets(slug, updateParams.flags.AbsoluteInstallBaseDir, updateParams.flags.Specs, updateParams.flags.ProjectDirAbs, updateParams.flags.IsGlobal)
	if err != nil {
		return err
	}

	targetVersion, err := common.ResolveSkillVersion(updateParams.serverDetails, updateParams.repoKey, slug, requestedVersion, updateParams.quiet)
	if err != nil {
		return err
	}

	results, err := updateSlugAcrossTargetsFn(updateParams, slug, targetVersion, targets)
	if err != nil {
		return err
	}
	if err := agentcommon.PrintInstallSummary("Skill", slug, targetVersion, results, updateParams.format); err != nil {
		return err
	}
	return finalError(results)
}

type updateAllOutcome struct {
	anyOK            bool
	anyFailed        bool
	firstResolveErr  error
	updatedSlugCount int
}

// confirmUpdateAll prompts before update --all (skipped for --dry-run, --quiet, and CI).
func confirmUpdateAll(updateParams update) error {
	if updateParams.dryRun || updateParams.quiet || isNonInteractive() {
		return nil
	}
	if !askYesNo(updateAllConfirmPrompt, false) {
		return fmt.Errorf("update --all aborted by user")
	}
	return nil
}

// runUpdateAll discovers installed skills under each --harness and updates each to its latest version.
func runUpdateAll(updateParams update) error {
	slugOrder, slugToTargets, err := discoverInstalledSkillTargets(updateParams.flags)
	if err != nil {
		return err
	}
	if len(slugOrder) == 0 {
		log.Info("No installed skills found for the given --harness list; nothing to update.")
		return nil
	}

	combined, outcome := applyUpdateAllForSlugs(updateParams, slugOrder, slugToTargets)
	return finalizeUpdateAll(combined, outcome, updateParams.format)
}

// discoverInstalledSkillTargets maps each installed slug to its harness install targets.
func discoverInstalledSkillTargets(flags agentcommon.InstallFlagsResult) ([]string, map[string][]common.AgentTarget, error) {
	slugToTargets := make(map[string][]common.AgentTarget)
	slugOrder := make([]string, 0)
	scope := common.ScopeProject
	if flags.IsGlobal {
		scope = common.ScopeGlobal
	}
	for _, spec := range flags.Specs {
		installDir, err := agentcommon.ResolveAgentInstallDir(spec, flags.ProjectDirAbs, flags.IsGlobal)
		if err != nil {
			return nil, nil, err
		}
		slugs, err := discoverInstalledSkillSlugs(installDir)
		if err != nil {
			return nil, nil, err
		}
		for _, slug := range slugs {
			if _, seen := slugToTargets[slug]; !seen {
				slugOrder = append(slugOrder, slug)
			}
			slugToTargets[slug] = append(slugToTargets[slug], common.AgentTarget{
				Agent:          spec,
				Scope:          scope,
				DestinationDir: filepath.Join(installDir, slug),
			})
		}
	}
	return slugOrder, slugToTargets, nil
}

// discoverInstalledSkillSlugs lists skill folder names under an install directory that have SKILL.md.
func discoverInstalledSkillSlugs(installDir string) ([]string, error) {
	entries, readErr := os.ReadDir(installDir)
	if readErr != nil && errors.Is(readErr, os.ErrNotExist) {
		return nil, nil
	}
	slugs := skillSlugsFromInstallDirEntries(installDir, entries)
	if readErr != nil {
		return slugs, fmt.Errorf("read install dir %s: %w", installDir, readErr)
	}
	return slugs, nil
}

// skillSlugsFromInstallDirEntries returns sorted slugs for directories that look like installed skills.
func skillSlugsFromInstallDirEntries(installDir string, entries []os.DirEntry) []string {
	slugs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillDir := filepath.Join(installDir, name)
		if _, err := publish.ReadInstalledSkillVersion(skillDir); err != nil {
			continue
		}
		slugs = append(slugs, name)
	}
	sort.Strings(slugs)
	return slugs
}

// applyUpdateAllForSlugs resolves latest version per slug, updates targets, and builds combined summary rows.
func applyUpdateAllForSlugs(updateParams update, slugOrder []string, slugToTargets map[string][]common.AgentTarget,
) ([]agentcommon.UpdateAllSummaryRow, updateAllOutcome) {
	combined := make([]agentcommon.UpdateAllSummaryRow, 0)
	var outcome updateAllOutcome
	for _, slug := range slugOrder {
		targetVersion, err := resolveLatestSkillVersion(updateParams.serverDetails, updateParams.repoKey, slug)
		if err != nil {
			if outcome.firstResolveErr == nil {
				outcome.firstResolveErr = err
			}
			log.Warn(fmt.Sprintf("Skipping skill '%s': could not resolve latest version: %s", slug, err.Error()))
			results := failedRowsForTargets(slugToTargets[slug], err.Error())
			combined = agentcommon.AppendUpdateAllSummaryRows(combined, slug, "", results)
			outcome.updatedSlugCount++
			_, slugFailed := tallySummaryRows(results)
			outcome.anyFailed = outcome.anyFailed || slugFailed
			continue
		}
		results, err := updateSlugAcrossTargetsFn(updateParams, slug, targetVersion, slugToTargets[slug])
		if err != nil {
			log.Warn(fmt.Sprintf("Skipping skill '%s': download failed: %s", slug, err.Error()))
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

// failedRowsForTargets builds a failed summary row for every target with the same detail message.
func failedRowsForTargets(targets []common.AgentTarget, detail string) []agentcommon.SummaryRow {
	rows := make([]agentcommon.SummaryRow, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, summaryRowFor(target, agentcommon.SummaryStatusFailed, detail))
	}
	return rows
}

// tallySummaryRows reports whether any per-target row succeeded or failed (skipped rows are ignored).
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

// finalizeUpdateAll prints the combined --all summary and returns an error if every update failed.
func finalizeUpdateAll(combined []agentcommon.UpdateAllSummaryRow, outcome updateAllOutcome, format string) error {
	if err := agentcommon.PrintUpdateAllSummary("Skill", combined, format); err != nil {
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

// updateSlugAcrossTargets downloads the skill once and runs the backup+copy loop per target.
func updateSlugAcrossTargets(updateParams update, slug, targetVersion string, targets []common.AgentTarget) ([]agentcommon.SummaryRow, error) {
	checks := preUpdateTargets(targets, targetVersion, updateParams.force, updateParams.quiet)
	results, updatable := initialResultsAndUpdatable(checks, targetVersion)

	if updateParams.dryRun {
		logDryRun(slug, targetVersion, checks)
		return results, nil
	}
	if len(updatable) == 0 {
		return results, nil
	}

	tmpDir, err := os.MkdirTemp("", "skill-update-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	installCmd := install.NewInstallCommand().
		SetServerDetails(updateParams.serverDetails).
		SetRepoKey(updateParams.repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetQuiet(updateParams.quiet).
		SetSuppressSummary(true).
		SetProjectDir(updateParams.flags.ProjectDirAbs).
		SetGlobal(updateParams.flags.IsGlobal)

	unzipDir, err := installCmd.FetchAndExtractTo(tmpDir)
	if err != nil {
		return nil, err
	}

	for _, preUpdateCheck := range updatable {
		results = append(results, updateOneSkill(unzipDir, installCmd, preUpdateCheck))
	}
	return results, nil
}

// preUpdateTargets checks each target for install presence and whether it is already at the target version.
func preUpdateTargets(targets []common.AgentTarget, targetVersion string, force, quiet bool) []preUpdate {
	out := make([]preUpdate, 0, len(targets))
	for _, agentTarget := range targets {
		record := preUpdate{agentTarget: agentTarget}
		installedVersion, err := publish.ReadInstalledSkillVersion(agentTarget.DestinationDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				record.failureReason = fmt.Sprintf("skill not installed at %s; run 'jf agent skills install' first", agentTarget.DestinationDir)
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

// initialResultsAndUpdatable splits pre-checks into summary rows and targets that need a download.
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

// summaryRowFor builds one install-summary row for an agent target.
func summaryRowFor(agentTarget common.AgentTarget, status, detail string) agentcommon.SummaryRow {
	return agentcommon.SummaryRow{
		Agent:  agentTarget.Agent.Name,
		Scope:  string(agentTarget.Scope),
		Path:   agentTarget.DestinationDir,
		Status: status,
		Detail: detail,
	}
}

// logDryRun logs what would happen for each target without changing the filesystem.
func logDryRun(slug, targetVersion string, checks []preUpdate) {
	for _, preUpdateCheck := range checks {
		switch {
		case preUpdateCheck.failureReason != "":
			log.Info(fmt.Sprintf("[dry-run] Would skip %s at %s: %s", slug, preUpdateCheck.agentTarget.DestinationDir, preUpdateCheck.failureReason))
		case preUpdateCheck.alreadyAtTargetVersion:
			log.Info(fmt.Sprintf("[dry-run] Skill '%s' already at v%s at %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		case preUpdateCheck.installedVersion == "":
			log.Info(fmt.Sprintf("[dry-run] Would install skill '%s' v%s to %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		default:
			log.Info(fmt.Sprintf("[dry-run] Would update skill '%s' from v%s -> v%s at %s", slug, preUpdateCheck.installedVersion, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		}
	}
}

// updateOneSkill backs up the current install, copies the new version, and restores on failure.
func updateOneSkill(unzipDir string, installCommand *install.InstallCommand, check preUpdate) agentcommon.SummaryRow {
	agentTarget := check.agentTarget
	slugBase := filepath.Base(agentTarget.DestinationDir)
	parent := filepath.Dir(agentTarget.DestinationDir)

	backupPath, err := reserveUpdateBackupPath(parent, slugBase)
	if err != nil {
		return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, err.Error())
	}
	if err := os.Rename(agentTarget.DestinationDir, backupPath); err != nil {
		return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, fmt.Sprintf("could not move current skill aside for update: %s", err.Error()))
	}

	rows := installCommand.CopyExtractedToTargets(unzipDir, []common.AgentTarget{agentTarget})
	if len(rows) != 1 {
		_ = os.RemoveAll(agentTarget.DestinationDir)
		if restoreErr := os.Rename(backupPath, agentTarget.DestinationDir); restoreErr != nil {
			return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, fmt.Sprintf("internal error: unexpected copy result count; restore failed: %s", restoreErr.Error()))
		}
		return summaryRowFor(agentTarget, agentcommon.SummaryStatusFailed, "internal error: unexpected copy result count")
	}
	row := rows[0]
	if row.Status != agentcommon.SummaryStatusOK {
		_ = os.RemoveAll(agentTarget.DestinationDir)
		if restoreErr := os.Rename(backupPath, agentTarget.DestinationDir); restoreErr != nil {
			row.Detail = fmt.Sprintf("%s; could not restore previous install: %s", row.Detail, restoreErr.Error())
		}
		return row
	}

	if err := os.RemoveAll(backupPath); err != nil {
		log.Warn(fmt.Sprintf("Update succeeded but previous copy at %s could not be deleted: %s", backupPath, err.Error()))
	} else {
		backupRoot := filepath.Join(parent, skillBackupDirName)
		_ = os.Remove(backupRoot)
	}

	return summaryRowFor(agentTarget, agentcommon.SummaryStatusOK, agentcommon.SummaryDetailOKInstall)
}

// finalError returns an error when every target in the summary failed.
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

// reserveUpdateBackupPath creates a unique backup directory path under .skill-backup for one slug.
func reserveUpdateBackupPath(installBase, slug string) (string, error) {
	backupRoot := filepath.Join(installBase, skillBackupDirName)
	// #nosec G301 -- update backup dir under user skill tree; permissive mode matches install copy behavior for tooling access.
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s directory: %w", skillBackupDirName, err)
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
