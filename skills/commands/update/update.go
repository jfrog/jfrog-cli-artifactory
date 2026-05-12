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

type preflight struct {
	target       common.AgentTarget
	installedVer string
	upToDate     bool
	failure      string
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

	checks := preflightTargets(targets, targetVersion, force, quiet)
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

	ic := install.NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetQuiet(quiet).
		SetSuppressSummary(true)

	unzipDir, err := ic.FetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	for _, check := range updatable {
		results = append(results, updateOneWithPrepared(unzipDir, ic, check))
	}

	if err := install.PrintSummary(slug, targetVersion, results, format); err != nil {
		return err
	}
	return finalError(results)
}

func preflightTargets(targets []common.AgentTarget, targetVersion string, force, quiet bool) []preflight {
	out := make([]preflight, 0, len(targets))
	for _, target := range targets {
		p := preflight{target: target}
		installedVer, err := readInstalledVersion(target.DestinationDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				p.failure = fmt.Sprintf("skill not installed at %s; run 'jf skills install' first", target.DestinationDir)
			} else {
				p.failure = err.Error()
			}
			if !quiet {
				log.Info(fmt.Sprintf("Skipping update for agent %s at %s: %s", target.Agent.Name, target.DestinationDir, p.failure))
			}
			out = append(out, p)
			continue
		}
		p.installedVer = installedVer
		if installedVer == targetVersion && !force {
			p.upToDate = true
			if !quiet {
				log.Info(fmt.Sprintf("Skipping update for agent %s at %s: already at version %s (use --force to re-download)", target.Agent.Name, target.DestinationDir, targetVersion))
			}
		}
		out = append(out, p)
	}
	return out
}

func initialResultsAndUpdatable(checks []preflight, targetVersion string) ([]install.SummaryRow, []preflight) {
	results := make([]install.SummaryRow, 0, len(checks))
	updatable := make([]preflight, 0, len(checks))
	for _, chk := range checks {
		switch {
		case chk.failure != "":
			results = append(results, summaryRowFor(chk.target, install.SummaryStatusFailed, chk.failure))
		case chk.upToDate:
			results = append(results, summaryRowFor(chk.target, install.SummaryStatusUpToDate, fmt.Sprintf("version already %s; use --force to reinstall", targetVersion)))
		default:
			updatable = append(updatable, chk)
		}
	}
	return results, updatable
}

func summaryRowFor(target common.AgentTarget, status, detail string) install.SummaryRow {
	return install.SummaryRow{
		Agent:  target.Agent.Name,
		Scope:  string(target.Scope),
		Path:   target.DestinationDir,
		Status: status,
		Detail: detail,
	}
}

func logDryRun(slug, targetVersion string, checks []preflight) {
	for _, c := range checks {
		switch {
		case c.failure != "":
			log.Info(fmt.Sprintf("[dry-run] Would skip %s at %s: %s", slug, c.target.DestinationDir, c.failure))
		case c.upToDate:
			log.Info(fmt.Sprintf("[dry-run] Skill '%s' already at v%s at %s", slug, targetVersion, c.target.DestinationDir))
		case c.installedVer == "":
			log.Info(fmt.Sprintf("[dry-run] Would install skill '%s' v%s to %s", slug, targetVersion, c.target.DestinationDir))
		default:
			log.Info(fmt.Sprintf("[dry-run] Would update skill '%s' from v%s -> v%s at %s", slug, c.installedVer, targetVersion, c.target.DestinationDir))
		}
	}
}

func updateOneWithPrepared(unzipDir string, ic *install.InstallCommand, check preflight) install.SummaryRow {
	target := check.target
	slugBase := filepath.Base(target.DestinationDir)
	parent := filepath.Dir(target.DestinationDir)

	backupPath, err := reserveUpdateBackupPath(parent, slugBase)
	if err != nil {
		return summaryRowFor(target, install.SummaryStatusFailed, err.Error())
	}
	if err := os.Rename(target.DestinationDir, backupPath); err != nil {
		return summaryRowFor(target, install.SummaryStatusFailed, fmt.Sprintf("could not move current skill aside for update: %s", err.Error()))
	}

	rows := ic.CopyExtractedToAgentTargets(unzipDir, []common.AgentTarget{target})
	if len(rows) != 1 {
		_ = os.RemoveAll(target.DestinationDir)
		if restoreErr := os.Rename(backupPath, target.DestinationDir); restoreErr != nil {
			return summaryRowFor(target, install.SummaryStatusFailed, fmt.Sprintf("internal error: unexpected copy result count; restore failed: %s", restoreErr.Error()))
		}
		return summaryRowFor(target, install.SummaryStatusFailed, "internal error: unexpected copy result count")
	}
	row := rows[0]
	if row.Status != install.SummaryStatusOK {
		_ = os.RemoveAll(target.DestinationDir)
		if restoreErr := os.Rename(backupPath, target.DestinationDir); restoreErr != nil {
			row.Detail = fmt.Sprintf("%s; could not restore previous install: %s", row.Detail, restoreErr.Error())
		}
		return row
	}

	if err := os.RemoveAll(backupPath); err != nil {
		log.Warn(fmt.Sprintf("Update succeeded but previous copy at %s could not be deleted: %s", backupPath, err.Error()))
	}

	return summaryRowFor(target, install.SummaryStatusOK, install.SummaryDetailOKInstall)
}

func finalError(results []install.SummaryRow) error {
	if len(results) == 0 {
		return nil
	}
	for _, r := range results {
		if r.Status != install.SummaryStatusFailed {
			return nil
		}
	}
	return fmt.Errorf("update failed for all targets (see summary above)")
}

func reserveUpdateBackupPath(installBase, slug string) (string, error) {
	pattern := fmt.Sprintf(".%s.jfrog-update-backup-*", slug)
	d, err := os.MkdirTemp(installBase, pattern)
	if err != nil {
		return "", fmt.Errorf("could not reserve update backup path: %w", err)
	}
	if err := os.Remove(d); err != nil {
		return "", fmt.Errorf("could not prepare update backup path: %w", err)
	}
	return d, nil
}

func readInstalledVersion(skillDir string) (string, error) {
	meta, err := publish.ParseSkillMeta(skillDir)
	if err != nil {
		return "", err
	}
	return meta.Version, nil
}
