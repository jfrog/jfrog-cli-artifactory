package common

import (
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type repoXrayConfig struct {
	XrayIndex *bool `json:"xrayIndex,omitempty"`
}

// WarnIfXrayDisabled fetches the repository configuration and warns if
// Xray indexing is not enabled, indicating security scanning is deactivated.
// The check is skipped entirely when JFROG_CLI_DISABLE_SKILLS_SCAN is set
// to "true" or "1".
func WarnIfXrayDisabled(serverDetails *config.ServerDetails, repoKey string) {
	if v := os.Getenv("JFROG_CLI_DISABLE_SKILLS_SCAN"); strings.EqualFold(v, "true") || v == "1" {
		log.Debug("Xray index check skipped (JFROG_CLI_DISABLE_SKILLS_SCAN is set)")
		return
	}

	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		log.Debug("Could not check repo xray config:", err.Error())
		return
	}
	var cfg repoXrayConfig
	if err := sm.GetRepository(repoKey, &cfg); err != nil {
		log.Debug("Could not fetch repo details:", err.Error())
		return
	}
	if cfg.XrayIndex == nil || !*cfg.XrayIndex {
		log.Warn("Preview version - security scanning is deactivated")
	}
}

func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]services.SkillVersion, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.ListSkillVersions(repoKey, slug)
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkills(repoKey, query, limit)
}

func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	return sm.SkillVersionExists(repoKey, slug, version)
}
