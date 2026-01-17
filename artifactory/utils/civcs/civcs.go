package civcs

import (
	"strings"

	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
)

// CI VCS property keys used by Maven/Gradle extractors
const (
	VcsProviderKey = "vcs.provider"
	VcsOrgKey      = "vcs.org"
	VcsRepoKey     = "vcs.repo"
)

// GetCIVcsPropsString returns CI VCS props if running in a CI environment, empty string otherwise.
// Returns format: "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo"
func GetCIVcsPropsString() string {
	info := cienv.GetCIVcsInfo()
	if info.IsEmpty() {
		return ""
	}
	result := BuildCIVcsPropsString(info)
	if result != "" {
		log.Debug("CI VCS properties detected:", result)
	}
	return result
}

// BuildCIVcsPropsString constructs the properties string from CI VCS info.
func BuildCIVcsPropsString(info cienv.CIVcsInfo) string {
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
	return strings.Join(parts, ";")
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
	if info.Provider != "" && !hasProp(userProps, "vcs.provider") {
		ciParts = append(ciParts, "vcs.provider="+info.Provider)
	}
	if info.Org != "" && !hasProp(userProps, "vcs.org") {
		ciParts = append(ciParts, "vcs.org="+info.Org)
	}
	if info.Repo != "" && !hasProp(userProps, "vcs.repo") {
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

// hasProp checks if the property key is already present in the semicolon-separated props string.
func hasProp(props, key string) bool {
	target := key + "="
	for _, prop := range strings.Split(props, ";") {
		if strings.HasPrefix(prop, target) {
			return true
		}
	}
	return false
}

// SetCIVcsPropsToConfig sets CI VCS properties to viper config if running in CI environment.
// These are picked up by the Maven/Gradle extractor and set as properties on deployed artifacts.
// Respects user precedence: if a property is already set, it is NOT overridden.
func SetCIVcsPropsToConfig(vConfig *viper.Viper) {
	ciVcsInfo := cienv.GetCIVcsInfo()
	if ciVcsInfo.IsEmpty() {
		return
	}
	log.Debug("Setting CI VCS properties for extractor: provider=", ciVcsInfo.Provider, ", org=", ciVcsInfo.Org, ", repo=", ciVcsInfo.Repo)
	if ciVcsInfo.Provider != "" && !vConfig.IsSet(VcsProviderKey) {
		vConfig.Set(VcsProviderKey, ciVcsInfo.Provider)
	}
	if ciVcsInfo.Org != "" && !vConfig.IsSet(VcsOrgKey) {
		vConfig.Set(VcsOrgKey, ciVcsInfo.Org)
	}
	if ciVcsInfo.Repo != "" && !vConfig.IsSet(VcsRepoKey) {
		vConfig.Set(VcsRepoKey, ciVcsInfo.Repo)
	}
}
