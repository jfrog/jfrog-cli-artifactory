package common

import (
	"fmt"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// SelectPackageVersion resolves "" / "latest" / exact match / interactive prompt for install and update.
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
