package common

import (
	"fmt"
	"strings"

	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
)

func GetServerDetails(c *components.Context) (*config.ServerDetails, error) {
	var details *config.ServerDetails
	var err error

	if hasServerConfigFlags(c) {
		details, err = pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	} else {
		details, err = config.GetDefaultServerConf()
	}
	if err != nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --url and --access-token flags: %w", err)
	}
	if details.ArtifactoryUrl == "" && details.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}

	normalizeArtifactoryUrl(details)
	return details, nil
}

// normalizeArtifactoryUrl ensures ArtifactoryUrl ends with /artifactory/.
// The --url flag is documented as "JFrog Platform URL" (e.g. https://acme.jfrog.io)
// but CreateArtifactoryDetailsByFlags copies it directly to ArtifactoryUrl without
// appending the /artifactory/ context path. This normalisation keeps our skills API
// calls consistent regardless of whether the user passes a platform URL or a full
// Artifactory URL.
func normalizeArtifactoryUrl(details *config.ServerDetails) {
	if details.ArtifactoryUrl == "" {
		return
	}
	artUrl := clientutils.AddTrailingSlashIfNeeded(details.ArtifactoryUrl)
	if !strings.Contains(artUrl, "/artifactory/") {
		artUrl += "artifactory/"
	}
	details.ArtifactoryUrl = artUrl

	if details.Url == "" {
		details.Url = strings.TrimSuffix(artUrl, "artifactory/")
	}
}

func hasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
}
