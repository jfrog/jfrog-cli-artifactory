package nuget

import (
	"fmt"
	"os"
	"path/filepath"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	dotnetcmd "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// WriteTempNuGetConfig creates a temporary directory containing a nuget.config file
// populated with the Artifactory NuGet feed URL and credentials derived from serverDetails.
// The returned cleanup function removes the temp directory; call it with defer.
func WriteTempNuGetConfig(serverDetails *config.ServerDetails, repoName string, useV2, allowInsecure bool) (configPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "jfrog-nuget-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir for nuget.config: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	sourceURL, user, password, err := dotnetcmd.GetSourceDetails(serverDetails, repoName, useV2)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	protocolVersion := "3"
	if useV2 {
		protocolVersion = "2"
	}

	content := fmt.Sprintf(dotnetutils.ConfigFileFormat,
		sourceURL, protocolVersion, allowInsecure, user, password)

	configPath = filepath.Join(tmpDir, "nuget.config")
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write temp nuget.config: %w", err)
	}
	return configPath, cleanup, nil
}
