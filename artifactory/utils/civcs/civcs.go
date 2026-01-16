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
// If user already specified vcs.provider, vcs.org, or vcs.repo, those are NOT overridden.
func MergeWithUserProps(userProps string) string {
	ciProps := GetCIVcsPropsString()
	if ciProps == "" || containsVcsProps(userProps) {
		return userProps
	}
	if userProps == "" {
		return ciProps
	}
	return userProps + ";" + ciProps
}

// containsVcsProps checks if the props string contains any vcs.provider, vcs.org, or vcs.repo.
func containsVcsProps(props string) bool {
	lower := strings.ToLower(props)
	return strings.Contains(lower, "vcs.provider=") ||
		strings.Contains(lower, "vcs.org=") ||
		strings.Contains(lower, "vcs.repo=")
}
