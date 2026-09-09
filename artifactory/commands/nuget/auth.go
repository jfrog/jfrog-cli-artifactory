package nuget

import (
	dotnetcmd "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// NuGetExeV3SourceDetails returns the V3 source URL, username and password for the temp
// nuget.config FlexPack writes. V3 is safe here because the URL goes into <packageSources> in
// the config file rather than onto the command line as -Source: with no -Source flag, nuget.exe
// does not re-embed the URL into MSBuild's /p:RestoreSources, and MSBuild instead reads the
// source from the config file via /p:RestoreConfigFile and loads the V3 service index normally.
//
// Both restore and push use this: the service index is what advertises PackagePublish and
// SymbolPackagePublish, so a V2 source would leave the native client with nowhere to send a
// symbol package.
func NuGetExeV3SourceDetails(serverDetails *config.ServerDetails, repoName string) (sourceURL, user, password string, err error) {
	return dotnetcmd.GetSourceDetails(serverDetails, repoName, false /* V3 */)
}
