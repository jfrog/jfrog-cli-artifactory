package civcs

import (
	"strings"

	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// GetCIVcsPropsString returns CI VCS props if running in a CI environment, empty string otherwise.
// Returns format: "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo"
func GetCIVcsPropsString() string {
	info := cienv.GetCIVcsInfo()
	if info.IsEmpty() {
		return ""
	}
	var parts []string
	if info.Provider != "" {
		parts = append(parts, "vcs.provider="+info.Provider)
	}
	if info.Org != "" {
		parts = append(parts, "vcs.org="+info.Org)
	}
	if info.Repo != "" {
		parts = append(parts, "vcs.repo="+info.Repo)
	}
	result := strings.Join(parts, ";")
	if result != "" {
		log.Debug("CI VCS properties detected:", result)
	}
	return result
}

// MergeWithUserProps adds CI VCS props to user-provided props, respecting user precedence.
// Only adds CI properties that the user hasn't already specified.
// For example, if user set vcs.org, we still add vcs.provider and vcs.repo from CI.
func MergeWithUserProps(userProps string) string {
	info := cienv.GetCIVcsInfo()
	if info.IsEmpty() {
		return userProps
	}
	var ciParts []string
	// Only add CI properties that user hasn't specified (case-sensitive)
	if info.Provider != "" && !strings.Contains(userProps, "vcs.provider=") {
		ciParts = append(ciParts, "vcs.provider="+info.Provider)
	}
	if info.Org != "" && !strings.Contains(userProps, "vcs.org=") {
		ciParts = append(ciParts, "vcs.org="+info.Org)
	}
	if info.Repo != "" && !strings.Contains(userProps, "vcs.repo=") {
		ciParts = append(ciParts, "vcs.repo="+info.Repo)
	}
	if len(ciParts) == 0 {
		return userProps
	}
	ciProps := strings.Join(ciParts, ";")
	if ciProps != "" {
		log.Debug("CI VCS properties to add:", ciProps)
	}
	if userProps == "" {
		return ciProps
	}
	return userProps + ";" + ciProps
}
