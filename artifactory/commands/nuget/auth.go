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

// NuGetExeV3SourceDetails returns the V3 source URL, username, and password for use in a
// temp nuget.config for restore operations. V3 is safe here because the URL goes into
// <packageSources> in the config file, not as a -Source CLI flag. When no -Source flag is
// passed, nuget.exe does NOT re-embed the URL into MSBuild's /p:RestoreSources — MSBuild
// reads the source directly from the config file via /p:RestoreConfigFile and loads the V3
// service index normally.
func NuGetExeV3SourceDetails(serverDetails *config.ServerDetails, repoName string) (sourceURL, user, password string, err error) {
	return dotnetcmd.GetSourceDetails(serverDetails, repoName, false /* V3 */)
}

// NuGetExeV2SourceDetails returns the V2 source URL, username, and password for push
// operations. Push is handled by nuget.exe directly (not MSBuild), so named source lookup
// from -ConfigFile works. V2 is fine for push (no service index needed) and avoids the
// /p:RestoreSources re-embedding issue entirely.
func NuGetExeV2SourceDetails(serverDetails *config.ServerDetails, repoName string) (sourceURL, user, password string, err error) {
	return dotnetcmd.GetSourceDetails(serverDetails, repoName, true /* V2 */)
}
