package nuget

import (
	"fmt"
	"net/url"

	dotnetcmd "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// SourceURLWithCredentials builds an Artifactory NuGet feed URL with credentials embedded
// as https://user:password@host/... so they can be passed directly as a -Source flag.
// This is rank-1 (command-line flag) in NuGet's credential priority hierarchy and requires
// no nuget.config modification.
func SourceURLWithCredentials(serverDetails *config.ServerDetails, repoName string, useV2 bool) (string, error) {
	sourceURL, user, password, err := dotnetcmd.GetSourceDetails(serverDetails, repoName, useV2)
	if err != nil {
		return "", fmt.Errorf("get NuGet source details: %w", err)
	}

	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("parse NuGet source URL: %w", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}
