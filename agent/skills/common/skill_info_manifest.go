package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

// SkillInfoManifestFile is the install manifest filename under .jfrog/.
const SkillInfoManifestFile = "skill-info.json"

// SkillInfoManifest is CLI-owned metadata for an installed skill (single source of truth for list/update).
type SkillInfoManifest = agentcommon.InstallInfoManifest
