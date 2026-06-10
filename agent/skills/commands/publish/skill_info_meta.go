package publish

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ReadInstalledSkillVersion returns the version string for an installed skill directory.
// It prefers .jfrog/skill-info.json (installedVersion) when present and non-empty,
// otherwise the version from SKILL.md front matter.
func ReadInstalledSkillVersion(skillDir string) (string, error) {
	manifest, err := agentcommon.ReadInstallInfoManifest(skillDir, common.SkillInfoManifestFile)
	if err != nil {
		log.Warn(fmt.Sprintf("Invalid skill-info manifest under %s (%v); using SKILL.md for installed version.", skillDir, err))
	} else if manifest != nil && strings.TrimSpace(manifest.InstalledVersion) != "" {
		return strings.TrimSpace(manifest.InstalledVersion), nil
	}
	meta, err := ParseSkillMeta(skillDir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(meta.Version), nil
}
