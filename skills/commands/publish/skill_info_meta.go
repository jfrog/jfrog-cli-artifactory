package publish

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
)

// ReadInstalledSkillVersion returns the version string for an installed skill directory.
// It prefers .jfrog/skill-info.json (installedVersion) when present and non-empty,
// otherwise the version from SKILL.md front matter.
func ReadInstalledSkillVersion(skillDir string) (string, error) {
	manifest, err := common.ReadSkillInfoManifest(skillDir)
	if err != nil {
		return "", fmt.Errorf("read skill info manifest: %w", err)
	}
	if manifest != nil && strings.TrimSpace(manifest.InstalledVersion) != "" {
		return strings.TrimSpace(manifest.InstalledVersion), nil
	}
	meta, err := ParseSkillMeta(skillDir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(meta.Version), nil
}
