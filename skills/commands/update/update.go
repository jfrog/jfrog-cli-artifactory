package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// skillBackupDirName is the directory under the skills parent where update backups are stored.
const skillBackupDirName = ".skill-backup"

type preUpdate struct {
	agentTarget            common.AgentTarget
	installedVersion       string
	alreadyAtTargetVersion bool
	failureReason          string
}

// RunUpdate is the CLI action for `jf skills update`.
func RunUpdate(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills update <slug> (--agent <name[,name...]> [--global] [--project-dir <dir>]] | --path <dir>) [--repo <repo>] [--version <ver>] [--dry-run] [--force] [--format <table|json>]")
	}

	slug := c.GetArgumentAt(0)
	if err := publish.ValidateSlug(slug); err != nil {
		return err
	}

	absoluteInstallBaseDir, specs, projectDirAbs, isGlobal, err := common.ValidateInstallFlags(c)
	if err != nil {
		return err
	}

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}
	quiet := common.IsQuiet(c)
	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	requestedVersion := c.GetStringFlagValue("version")
	dryRun := c.GetBoolFlagValue("dry-run")
	force := c.GetBoolFlagValue("force")
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	targets, err := common.ResolveAgentTargets(slug, absoluteInstallBaseDir, specs, projectDirAbs, isGlobal)
	if err != nil {
		return err
	}

	targetVersion, err := common.ResolveSkillVersion(serverDetails, repoKey, slug, requestedVersion, quiet)
	if err != nil {
		return err
	}

	checks := preUpdateTargets(targets, targetVersion, force, quiet)
	results, updatable := initialResultsAndUpdatable(checks, targetVersion)

	if dryRun {
		logDryRun(slug, targetVersion, checks)
		return install.PrintSummary(slug, targetVersion, results, format)
	}

	if len(updatable) == 0 {
		if err := install.PrintSummary(slug, targetVersion, results, format); err != nil {
			return err
		}
		return finalError(results)
	}

	tmpDir, err := os.MkdirTemp("", "skill-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cmd := install.NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetQuiet(quiet).
		SetSuppressSummary(true).
		SetProjectDir(projectDirAbs).
		SetGlobal(isGlobal)

	unzipDir, err := cmd.FetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	for _, preUpdateCheck := range updatable {
		results = append(results, updateOneSkill(unzipDir, cmd, preUpdateCheck))
	}

	if err := install.PrintSummary(slug, targetVersion, results, format); err != nil {
		return err
	}
	return finalError(results)
}

func preUpdateTargets(targets []common.AgentTarget, targetVersion string, force, quiet bool) []preUpdate {
	out := make([]preUpdate, 0, len(targets))
	for _, agentTarget := range targets {
		record := preUpdate{agentTarget: agentTarget}
		installedVersion, err := publish.ReadInstalledSkillVersion(agentTarget.DestinationDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				record.failureReason = fmt.Sprintf("skill not installed at %s; run 'jf skills install' first", agentTarget.DestinationDir)
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

func initialResultsAndUpdatable(checks []preUpdate, targetVersion string) ([]install.SummaryRow, []preUpdate) {
	results := make([]install.SummaryRow, 0, len(checks))
	updatable := make([]preUpdate, 0, len(checks))
	for _, preUpdateCheck := range checks {
		switch {
		case preUpdateCheck.failureReason != "":
			results = append(results, summaryRowFor(preUpdateCheck.agentTarget, install.SummaryStatusFailed, preUpdateCheck.failureReason))
		case preUpdateCheck.alreadyAtTargetVersion:
			results = append(results, summaryRowFor(preUpdateCheck.agentTarget, install.SummaryStatusSkipped, fmt.Sprintf("version already %s; use --force to reinstall", targetVersion)))
		default:
			updatable = append(updatable, preUpdateCheck)
		}
	}
	return results, updatable
}

func summaryRowFor(agentTarget common.AgentTarget, status, detail string) install.SummaryRow {
	return install.SummaryRow{
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
			log.Info(fmt.Sprintf("[dry-run] Skill '%s' already at v%s at %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		case preUpdateCheck.installedVersion == "":
			log.Info(fmt.Sprintf("[dry-run] Would install skill '%s' v%s to %s", slug, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		default:
			log.Info(fmt.Sprintf("[dry-run] Would update skill '%s' from v%s -> v%s at %s", slug, preUpdateCheck.installedVersion, targetVersion, preUpdateCheck.agentTarget.DestinationDir))
		}
	}
}

// updateOneSkill updates a single install target using the already-fetched tree in unzipDir:
// it renames the live install aside, copies from unzipDir, restores the backup on failure, then removes the backup on success.
func updateOneSkill(unzipDir string, installCommand *install.InstallCommand, check preUpdate) install.SummaryRow {
	agentTarget := check.agentTarget
	slugBase := filepath.Base(agentTarget.DestinationDir)
	parent := filepath.Dir(agentTarget.DestinationDir)

	backupPath, err := reserveUpdateBackupPath(parent, slugBase)
	if err != nil {
		return summaryRowFor(agentTarget, install.SummaryStatusFailed, err.Error())
	}
	if err := os.Rename(agentTarget.DestinationDir, backupPath); err != nil {
		return summaryRowFor(agentTarget, install.SummaryStatusFailed, fmt.Sprintf("could not move current skill aside for update: %s", err.Error()))
	}

	rows := installCommand.CopyExtractedToAgentTargets(unzipDir, []common.AgentTarget{agentTarget})
	if len(rows) != 1 {
		_ = os.RemoveAll(agentTarget.DestinationDir)
		if restoreErr := os.Rename(backupPath, agentTarget.DestinationDir); restoreErr != nil {
			return summaryRowFor(agentTarget, install.SummaryStatusFailed, fmt.Sprintf("internal error: unexpected copy result count; restore failed: %s", restoreErr.Error()))
		}
		return summaryRowFor(agentTarget, install.SummaryStatusFailed, "internal error: unexpected copy result count")
	}
	row := rows[0]
	if row.Status != install.SummaryStatusOK {
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

	return summaryRowFor(agentTarget, install.SummaryStatusOK, install.SummaryDetailOKInstall)
}

func finalError(results []install.SummaryRow) error {
	if len(results) == 0 {
		return nil
	}
	for _, result := range results {
		if result.Status != install.SummaryStatusFailed {
			return nil
		}
	}
	return fmt.Errorf("update failed for all targets (see summary above)")
}

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
