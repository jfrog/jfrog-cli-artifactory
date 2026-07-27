package apmcommon

import (
	"os/exec"
	"regexp"

	"github.com/jfrog/gofrog/version"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const minSupportedApmVersion = "0.1.0"

// apmVersionPattern extracts the dotted version number from apm's descriptive `--version`
// output, e.g. "Agent Package Manager (APM) CLI version 0.23.1 (d1d926d)" -> "0.23.1".
var apmVersionPattern = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

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

func GetApmVersion() (*version.Version, error) {
	out, err := exec.Command("apm", "--version").Output()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to determine apm version. Ensure apm is installed: %s", err.Error())
	}
	match := apmVersionPattern.FindString(string(out))
	if match == "" {
		return nil, errorutils.CheckErrorf("could not parse apm version from output: %s", string(out))
	}
	return version.NewVersion(match), nil
}
