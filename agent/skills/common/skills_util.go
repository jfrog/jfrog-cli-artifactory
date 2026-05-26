package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ResolveSkillVersion lists remote versions then applies SelectSkillVersion rules.
func ResolveSkillVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return "", fmt.Errorf("skill '%s' not found in repository '%s'", slug, repoKey)
		}
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	available := make([]string, len(versions))
	for idx, skillVersion := range versions {
		available[idx] = skillVersion.Version
	}
	return SelectSkillVersion(available, requested, repoKey, quiet)
}

// SelectSkillVersion resolves "" / "latest" / exact match / prompt.
func SelectSkillVersion(available []string, requested, repoKey string, quiet bool) (string, error) {
	if requested == "" || requested == "latest" {
		latest, err := agentcommon.LatestVersion(available)
		if err != nil {
			return "", fmt.Errorf("failed to determine latest version: %w", err)
		}
		log.Info(fmt.Sprintf("Using latest version: %s", latest))
		return latest, nil
	}

	for _, version := range available {
		if version == requested {
			return requested, nil
		}
	}

	if quiet || agentcommon.IsNonInteractive() {
		return "", fmt.Errorf(
			"version '%s' not found in repository '%s'.\nAvailable versions: %s",
			requested, repoKey, strings.Join(available, ", "),
		)
	}

	log.Warn(fmt.Sprintf("Version '%s' not found. Please select from the available versions below.", requested))

	options := make([]prompt.Suggest, len(available))
	for idx, version := range available {
		options[idx] = prompt.Suggest{Text: version}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		"Select a version:",
		fmt.Sprintf("'%s' is not in the list of available versions.", requested),
		options,
	)
	return selected, nil
}

// ValidateExistingDir requires path to exist and be a directory (after filepath.Abs).
func ValidateExistingDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	return nil
}

// ExpandHome maps ~/ to the user home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
