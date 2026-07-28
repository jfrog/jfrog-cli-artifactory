package apmcommon

import (
	"os/exec"
	"strings"

	"github.com/jfrog/gofrog/version"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const minSupportedApmVersion = "0.1.0"

// ValidateApmPrerequisites checks that apm is installed and meets minSupportedApmVersion.
func ValidateApmPrerequisites() error {
	ver, err := GetApmVersion()
	if err != nil {
		return err
	}
	if !ver.AtLeast(minSupportedApmVersion) {
		return errorutils.CheckErrorf(
			"JFrog CLI apm commands require apm version %s or higher. Current version: %s",
			minSupportedApmVersion, ver.GetVersion())
	}
	log.Debug("apm version:", ver.GetVersion())
	return nil
}

// GetApmVersion runs "apm --version" and parses the dotted version number out of its
// descriptive output.
func GetApmVersion() (*version.Version, error) {
	out, err := exec.Command("apm", "--version").Output()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to determine apm version. Ensure apm is installed: %s", err.Error())
	}
	match := parseApmVersion(string(out))
	if match == "" {
		return nil, errorutils.CheckErrorf("could not parse apm version from output: %s", string(out))
	}
	return version.NewVersion(match), nil
}

// parseApmVersion extracts the dotted version number from apm's descriptive `--version`
// output, e.g. "Agent Package Manager (APM) CLI version 0.23.1 (d1d926d)" -> "0.23.1".
// Returns "" if no whitespace-delimited token looks like a version number.
func parseApmVersion(output string) string {
	for field := range strings.FieldsSeq(output) {
		if isDottedVersion(field) {
			return field
		}
	}
	return ""
}

// isDottedVersion reports whether token is two or three dot-separated numeric segments,
// e.g. "1.2" or "0.23.1".
func isDottedVersion(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}
