package common

import (
	"fmt"
	"strings"

	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
)

// GetServerDetails returns ServerDetails from CLI flags when present, otherwise the default
// configured server. The Artifactory URL is normalized to always end with /artifactory/.
func GetServerDetails(commandContext *components.Context) (*config.ServerDetails, error) {
	var details *config.ServerDetails
	var err error

	if hasServerConfigFlags(commandContext) {
		details, err = pluginsCommon.CreateArtifactoryDetailsByFlags(commandContext)
	} else {
		details, err = config.GetDefaultServerConf()
	}
	if err != nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --url and --access-token flags: %w", err)
	}
	if details == nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --url and --access-token flags")
	}
	if details.ArtifactoryUrl == "" && details.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}
	NormalizeArtifactoryUrl(details)
	return details, nil
}

// GetServerDetailsByID returns ServerDetails for serverID, or the default configured server if
// serverID is empty. Unlike GetServerDetails, it never inspects commandContext flags - callers
// that manually extract --server-id from raw arguments (e.g. SkipFlagParsing subcommands, where
// urfave/cli parses none of jf's own flags) resolve it through this instead.
func GetServerDetailsByID(serverID string) (*config.ServerDetails, error) {
	details, err := config.GetSpecificConfig(serverID, true, false)
	if err != nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --server-id: %w", err)
	}
	if details == nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --server-id")
	}
	if details.ArtifactoryUrl == "" && details.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}
	NormalizeArtifactoryUrl(details)
	return details, nil
}

// NormalizeArtifactoryUrl ensures details.ArtifactoryUrl always ends with /artifactory/,
// filling in details.Url from it when Url is empty.
func NormalizeArtifactoryUrl(details *config.ServerDetails) {
	artifactoryURL := details.GetArtifactoryUrl()
	if artifactoryURL == "" {
		return
	}
	artifactoryURL = clientutils.AddTrailingSlashIfNeeded(artifactoryURL)
	if !strings.Contains(artifactoryURL, "/artifactory/") {
		artifactoryURL += "artifactory/"
	}
	details.ArtifactoryUrl = artifactoryURL

	if details.GetUrl() == "" {
		details.Url = strings.TrimSuffix(artifactoryURL, "artifactory/")
	}
}

func hasServerConfigFlags(commandContext *components.Context) bool {
	return commandContext.IsFlagSet("url") ||
		commandContext.IsFlagSet("user") ||
		commandContext.IsFlagSet("access-token") ||
		commandContext.IsFlagSet("server-id") ||
		(commandContext.IsFlagSet("password") && (commandContext.IsFlagSet("url") || commandContext.IsFlagSet("server-id")))
}
