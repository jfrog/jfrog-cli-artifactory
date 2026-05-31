package civcs

import (
	"fmt"
	"os"
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
	VcsUrlKey      = "vcs.url"
	VcsRevisionKey = "vcs.revision"
	VcsBranchKey   = "vcs.branch"

	// CIVcsPropsDisabledEnvVar is the environment variable that disables CI VCS property collection.
	// When set to "true", CI VCS properties will not be collected or set on artifacts.
	// This is primarily used for testing to prevent CI VCS props from interfering with
	// tests that check artifact properties.
	CIVcsPropsDisabledEnvVar = "JFROG_CLI_CI_VCS_PROPS_DISABLED"
)

// IsCIVcsPropsDisabled checks if CI VCS property collection is disabled via environment variable.
func IsCIVcsPropsDisabled() bool {
	return os.Getenv(CIVcsPropsDisabledEnvVar) == "true"
}

// GetCIVcsPropsString returns CI VCS props if running in a CI environment, empty string otherwise.
// Returns format: "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo"
// Returns empty string if CI VCS props collection is disabled via JFROG_CLI_CI_VCS_PROPS_DISABLED.
func GetCIVcsPropsString() string {
	if IsCIVcsPropsDisabled() {
		return ""
	}
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
		parts = append(parts, fmt.Sprintf("%s=%s", VcsProviderKey, info.Provider))
	}
	if info.Org != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", VcsOrgKey, info.Org))
	}
	if info.Repo != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", VcsRepoKey, info.Repo))
	}
	if info.Url != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", VcsUrlKey, info.Url))
	}
	if info.Revision != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", VcsRevisionKey, info.Revision))
	}
	if info.Branch != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", VcsBranchKey, info.Branch))
	}
	return strings.Join(parts, ";")
}

// MergeWithUserProps adds CI VCS props to user-provided props, respecting user precedence.
// Only adds CI properties that the user hasn't already specified.
// For example, if user set vcs.org, we still add vcs.provider and vcs.repo from CI.
// Returns userProps unchanged if CI VCS props collection is disabled via JFROG_CLI_CI_VCS_PROPS_DISABLED.
func MergeWithUserProps(userProps string) string {
	if IsCIVcsPropsDisabled() {
		return userProps
	}
	info := cienv.GetCIVcsInfo()
	if info.IsEmpty() {
		return userProps
	}
	return MergeVcsProps(userProps, info)
}

// MergeWithUserAndDetectedProps adds CI and local git VCS props to user-provided props.
// Precedence: user props > CI props > local git props.
func MergeWithUserAndDetectedProps(userProps, sourcePattern string) string {
	ciPropsMerged := MergeWithUserProps(userProps)
	if hasAllLocalGitProps(ciPropsMerged) || IsCIVcsPropsDisabled() {
		// All local git props are already present or CI VCS props collection is disabled, return merged props
		return ciPropsMerged
	}
	log.Debug("Local git VCS props not present in user props, getting from source pattern:", sourcePattern)
	localInfo, err := getLocalGitVcsInfo(sourcePattern)
	if err != nil {
		log.Debug("Skipping local git VCS props, failed getting VCS info:", err.Error())
		return ciPropsMerged
	}
	return MergeVcsProps(ciPropsMerged, localInfo)
}

func hasAllLocalGitProps(props string) bool {
	return hasProp(props, VcsUrlKey) && hasProp(props, VcsRevisionKey) && hasProp(props, VcsBranchKey)
}

func MergeVcsProps(userProps string, info cienv.CIVcsInfo) string {
	var newProps []string
	if info.Provider != "" && !hasProp(userProps, VcsProviderKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsProviderKey, info.Provider))
	}
	if info.Org != "" && !hasProp(userProps, VcsOrgKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsOrgKey, info.Org))
	}
	if info.Repo != "" && !hasProp(userProps, VcsRepoKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsRepoKey, info.Repo))
	}
	if info.Url != "" && !hasProp(userProps, VcsUrlKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsUrlKey, info.Url))
	}
	if info.Revision != "" && !hasProp(userProps, VcsRevisionKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsRevisionKey, info.Revision))
	}
	if info.Branch != "" && !hasProp(userProps, VcsBranchKey) {
		newProps = append(newProps, fmt.Sprintf("%s=%s", VcsBranchKey, info.Branch))
	}
	if len(newProps) == 0 {
		return userProps
	}
	props := strings.Join(newProps, ";")
	if props != "" {
		log.Debug("VCS properties to add:", props)
	}
	return userProps + ";" + props
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
// Does nothing if CI VCS props collection is disabled via JFROG_CLI_CI_VCS_PROPS_DISABLED.
func SetCIVcsPropsToConfig(vConfig *viper.Viper) {
	if IsCIVcsPropsDisabled() {
		return
	}
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
	if ciVcsInfo.Url != "" && !vConfig.IsSet(VcsUrlKey) {
		vConfig.Set(VcsUrlKey, ciVcsInfo.Url)
	}
	if ciVcsInfo.Revision != "" && !vConfig.IsSet(VcsRevisionKey) {
		vConfig.Set(VcsRevisionKey, ciVcsInfo.Revision)
	}
	if ciVcsInfo.Branch != "" && !vConfig.IsSet(VcsBranchKey) {
		vConfig.Set(VcsBranchKey, ciVcsInfo.Branch)
	}
}
