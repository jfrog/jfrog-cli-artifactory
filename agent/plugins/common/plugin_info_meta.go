package common

import (
	"errors"
	"fmt"
	"os"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ReadInstalledPluginVersion returns the version string for an installed plugin directory.
// It prefers .jfrog/plugin-info.json (installedVersion) when present and non-empty,
// otherwise falls back to the version from plugin.json.
// Returns an error wrapping os.ErrNotExist when the install directory or plugin.json
// is missing, so callers can use errors.Is(err, fs.ErrNotExist) to detect "not installed".
func ReadInstalledPluginVersion(pluginDir string) (string, error) {
	manifest, err := agentcommon.ReadInstallInfoManifest(pluginDir, PluginInfoManifestFile)
	if err != nil {
		log.Warn(fmt.Sprintf("Invalid plugin-info manifest under %s (%v); using plugin.json for installed version.", pluginDir, err))
	} else if manifest != nil && strings.TrimSpace(manifest.InstalledVersion) != "" {
		return strings.TrimSpace(manifest.InstalledVersion), nil
	}

	if _, statErr := os.Stat(pluginDir); statErr != nil {
		return "", statErr
	}

	_, meta, err := findPrimaryPluginManifest(pluginDir)
	if err != nil {
		if errors.Is(err, ErrPluginManifestNotFound) {
			return "", fmt.Errorf("%w: %s", os.ErrNotExist, err.Error())
		}
		return "", err
	}
	return strings.TrimSpace(meta.Version), nil
}
