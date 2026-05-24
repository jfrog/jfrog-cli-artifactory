package common

import (
	"fmt"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ListPackageVersions returns the version folder names under <repoKey>/<slug>/.
// Non-semver child folders and files are ignored. Backed by the generic storage API
// because the Artifactory client SDK has no typed list-versions call for agent plugins yet.
func ListPackageVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]string, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	folderInfo, err := serviceManager.FolderInfo(fmt.Sprintf("%s/%s", repoKey, slug))
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(folderInfo.Children))
	for _, child := range folderInfo.Children {
		if !child.Folder {
			continue
		}
		name := strings.TrimPrefix(child.Uri, "/")
		if name == "" {
			continue
		}
		if err := ValidateSemver(name); err != nil {
			continue
		}
		versions = append(versions, name)
	}
	return versions, nil
}

// ResolvePackageVersion lists remote versions then applies SelectPackageVersion rules.
func ResolvePackageVersion(serverDetails *config.ServerDetails, repoKey, slug, requested string, quiet bool) (string, error) {
	available, err := ListPackageVersions(serverDetails, repoKey, slug)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return "", fmt.Errorf("package '%s' not found in repository '%s'", slug, repoKey)
		}
		return "", fmt.Errorf("failed to list versions: %w", err)
	}
	return SelectPackageVersion(available, requested, repoKey, quiet)
}

// SelectPackageVersion resolves "" / "latest" / exact match / interactive prompt.
// In quiet or non-interactive mode an unknown requested version is an error.
func SelectPackageVersion(available []string, requested, repoKey string, quiet bool) (string, error) {
	if requested == "" || requested == "latest" {
		latest, err := LatestVersion(available)
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

	if quiet || IsNonInteractive() {
		return "", fmt.Errorf(
			"version '%s' not found in repository '%s'.\nAvailable versions: %s",
			requested, repoKey, strings.Join(available, ", "),
		)
	}

	log.Warn(fmt.Sprintf("Version '%s' not found. Please select from the available versions below.", requested))

	options := make([]prompt.Suggest, len(available))
	for index, version := range available {
		options[index] = prompt.Suggest{Text: version}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		"Select a version:",
		fmt.Sprintf("'%s' is not in the list of available versions.", requested),
		options,
	)
	return selected, nil
}
