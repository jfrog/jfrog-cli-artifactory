package common

import (
	"fmt"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const latestVersionKeyword = "latest"

// SelectPackageVersionOpts configures version resolution for install and update.
type SelectPackageVersionOpts struct {
	Available []string
	Requested string
	RepoKey   string
	Quiet     bool
}

// SelectPackageVersion resolves "" / "latest" / exact match / interactive prompt for install and update.
func SelectPackageVersion(opts SelectPackageVersionOpts) (string, error) {
	requested := strings.TrimSpace(opts.Requested)
	if isLatestVersionRequest(requested) {
		return selectLatestPackageVersion(opts.Available)
	}
	if version, found := findPackageVersion(opts.Available, requested); found {
		return version, nil
	}
	if opts.Quiet || IsNonInteractive() {
		return "", fmt.Errorf(
			"version '%s' not found in repository '%s'.\nAvailable versions: %s",
			requested, opts.RepoKey, strings.Join(opts.Available, ", "),
		)
	}
	return promptPackageVersionFromList(requested, opts.Available), nil
}

func isLatestVersionRequest(requested string) bool {
	return requested == "" || requested == latestVersionKeyword
}

func selectLatestPackageVersion(available []string) (string, error) {
	latest, err := LatestVersion(available)
	if err != nil {
		return "", fmt.Errorf("failed to determine latest version: %w", err)
	}
	log.Info(fmt.Sprintf("Using latest version: %s", latest))
	return latest, nil
}

func findPackageVersion(available []string, requested string) (string, bool) {
	for _, version := range available {
		if version == requested {
			return requested, true
		}
	}
	return "", false
}

func promptPackageVersionFromList(requested string, available []string) string {
	log.Warn(fmt.Sprintf("Version '%s' not found. Please select from the available versions below.", requested))

	options := make([]prompt.Suggest, len(available))
	for idx, version := range available {
		options[idx] = prompt.Suggest{Text: version}
	}
	return ioutils.AskFromListWithMismatchConfirmation(
		"Select a version:",
		fmt.Sprintf("'%s' is not in the list of available versions.", requested),
		options,
	)
}
