package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

// PluginInfoManifestFile is the install manifest filename under .jfrog/.
const PluginInfoManifestFile = "plugin-info.json"

// PluginInfoManifest is CLI-owned metadata for an installed plugin (single source of truth for list/update).
type PluginInfoManifest = agentcommon.InstallInfoManifest
