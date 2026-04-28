package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// RunUpdate is the CLI action for `jf skills update`.
func RunUpdate(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills update <slug> [--path <dir>] [--repo <repo>] [options]")
	}

	slug := c.GetArgumentAt(0)
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

	targetVersion, err = resolveTargetVersion(serverDetails, repoKey, slug, targetVersion, quiet)
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

	cmd := install.NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(targetVersion).
		SetInstallPath(installBase).
		SetQuiet(quiet).
		SetRemoveExisting(true)

	return cmd.Run()
}

// validateInstallBase checks that the base install directory exists.
func validateInstallBase(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path '%s' does not exist", path)
	}
	return nil
}

// readInstalledVersion reads the version from the skill's SKILL.md.
// Returns an empty string (no error) when SKILL.md has no version field.
// Returns an error if the skill directory or SKILL.md is missing.
func readInstalledVersion(skillDir string) (string, error) {
	meta, err := publish.ParseSkillMeta(skillDir)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "failed to read SKILL.md") {
			return "", fmt.Errorf(
				"skill '%s' is not installed at '%s'.\n"+
					"To install it, run: jf skills install %s --path %s",
				filepath.Base(skillDir), filepath.Dir(skillDir),
				filepath.Base(skillDir), filepath.Dir(skillDir),
			)
		}
		log.Warn(fmt.Sprintf("Could not read current version from %s/SKILL.md: %s", skillDir, err.Error()))
		return "", nil
	}
	return meta.Version, nil
}

// resolveTargetVersion validates the requested version against what is available in Artifactory.
//   - Empty / "latest": resolves to the latest semver.
//   - Explicit version that exists: returns it as-is.
//   - Explicit version that does NOT exist: prompts the user to pick from available versions
//     interactively, or returns a clear error listing available versions in quiet/non-interactive mode.
func resolveTargetVersion(serverDetails *config.ServerDetails, repoKey, slug, version string, quiet bool) (string, error) {
	versions, err := common.ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return "", fmt.Errorf("skill '%s' not found in repository '%s'", slug, repoKey)
		}
		return "", fmt.Errorf("failed to list versions: %w", err)
	}

	versionStrs := make([]string, len(versions))
	for i, v := range versions {
		versionStrs[i] = v.Version
	}

	return selectVersion(versionStrs, version, repoKey, quiet)
}

// selectVersion picks a version from the available list.
// Exported for testing.
func selectVersion(available []string, requested, repoKey string, quiet bool) (string, error) {
	if requested == "" || requested == "latest" {
		latest, err := common.LatestVersion(available)
		if err != nil {
			return "", fmt.Errorf("failed to determine latest version: %w", err)
		}
		log.Info(fmt.Sprintf("Using latest version: %s", latest))
		return latest, nil
	}

	for _, v := range available {
		if v == requested {
			return requested, nil
		}
	}

	// Version not found — prompt interactively or return a clear error.
	if quiet || common.IsNonInteractive() {
		return "", fmt.Errorf(
			"version '%s' not found in repository '%s'.\nAvailable versions: %s",
			requested, repoKey, strings.Join(available, ", "),
		)
	}

	log.Warn(fmt.Sprintf("Version '%s' not found. Please select from the available versions below.", requested))

	options := make([]prompt.Suggest, len(available))
	for i, v := range available {
		options[i] = prompt.Suggest{Text: v}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		"Select a version:",
		fmt.Sprintf("'%%s' is not in the list of available versions."),
		options,
	)
	return selected, nil
}
