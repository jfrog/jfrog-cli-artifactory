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

// RunUpdate is the CLI action for `jf skills update`.
func RunUpdate(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills update <slug> [--path <dir>] [--repo <repo>] [options]")
	}

	slug := c.GetArgumentAt(0)
	if err := publish.ValidateSlug(slug); err != nil {
		return err
	}

	installBase := c.GetStringFlagValue("path")
	if installBase == "" {
		installBase = "."
	}
	targetVersion := c.GetStringFlagValue("version")
	dryRun := c.GetBoolFlagValue("dry-run")
	force := c.GetBoolFlagValue("force")
	quiet := common.IsQuiet(c)

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	if err := validateInstallBase(installBase); err != nil {
		return err
	}

	skillDir := filepath.Join(installBase, slug)

	currentVersion, err := readInstalledVersion(skillDir)
	if err != nil {
		return err
	}

	targetVersion, err = common.ResolveSkillVersion(serverDetails, repoKey, slug, targetVersion, quiet)
	if err != nil {
		return err
	}

	if currentVersion == targetVersion && !force {
		log.Info(fmt.Sprintf("Skill '%s' is already at version '%s'. Use --force to re-download.", slug, currentVersion))
		return nil
	}

	if dryRun {
		if currentVersion == "" {
			log.Info(fmt.Sprintf("[dry-run] Would install skill '%s' v%s to %s", slug, targetVersion, skillDir))
		} else {
			log.Info(fmt.Sprintf("[dry-run] Would update skill '%s' from v%s -> v%s at %s", slug, currentVersion, targetVersion, skillDir))
		}
		return nil
	}

	backupPath, err := reserveUpdateBackupPath(installBase, slug)
	if err != nil {
		return err
	}
	if err := os.Rename(skillDir, backupPath); err != nil {
		return fmt.Errorf("could not move current skill aside for update: %w", err)
	}

	cmd := install.NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetInstallPath(installBase).
		SetQuiet(quiet)

	runErr := cmd.Run()
	if runErr != nil {
		_ = os.RemoveAll(skillDir)
		if rerr := os.Rename(backupPath, skillDir); rerr != nil {
			return fmt.Errorf("update failed (%w); could not restore previous install at %s: %w", runErr, skillDir, rerr)
		}
		return runErr
	}

	if err := os.RemoveAll(backupPath); err != nil {
		log.Warn(fmt.Sprintf("Update succeeded but previous copy at %s could not be deleted: %s", backupPath, err.Error()))
	}

	if currentVersion == "" {
		log.Info(fmt.Sprintf("Skill '%s' update completed: version '%s' at %s", slug, targetVersion, skillDir))
	} else {
		log.Info(fmt.Sprintf("Skill '%s' update completed: version '%s' -> '%s' at %s", slug, currentVersion, targetVersion, skillDir))
	}
	return nil
}

// reserveUpdateBackupPath returns a non-existent path under installBase used to hold the previous skill tree during update.
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

func validateInstallBase(path string) error {
	if err := common.ValidateExistingDir(path); err != nil {
		return fmt.Errorf("install path %q: %w", path, err)
	}
	return nil
}

func readInstalledVersion(skillDir string) (string, error) {
	meta, err := publish.ParseSkillMeta(skillDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf(
				"skill '%s' is not installed at '%s'.\n"+
					"To install it, run: jf skills install %s --path %s",
				filepath.Base(skillDir), filepath.Dir(skillDir),
				filepath.Base(skillDir), filepath.Dir(skillDir),
			)
		}
		return "", fmt.Errorf("could not read installed skill metadata from %s: %w", skillDir, err)
	}
	return meta.Version, nil
}
