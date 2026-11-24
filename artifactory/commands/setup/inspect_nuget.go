package setup

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	bidotnet "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// getNugetSourceURL is replaced in tests so they do not need nuget or dotnet installed.
var getNugetSourceURL = dotnet.GetNugetSourceURL

func nugetRepoKey(rest string) string {
	if repoKey := repoKeyAfter(rest, "api", "nuget", "v3"); repoKey != "" {
		return repoKey
	}
	return repoKeyAfter(rest, "api", "nuget")
}

func inspectNuget(packageManager project.ProjectType, serverDetails *config.ServerDetails) (inspection, error) {
	toolchain, found := bidotnet.DotnetCore, binaryFound("dotnet")
	if packageManager == project.Nuget {
		// On Linux, build-info-go also runs NuGet as nuget.exe under Mono.
		toolchain, found = bidotnet.Nuget, binaryFound("nuget") || (runtime.GOOS == "linux" && binaryFound("mono") && binaryFound("nuget.exe"))
	}
	result := newInspection(found)
	// NuGet merges several config files by rules only the tool applies, so the source is
	// read through the tool, and without it there is nothing reliable to report.
	if !found {
		return result, nil
	}
	sourceURL, enabled, err := getNugetSourceURL(toolchain)
	if err != nil {
		return inspection{}, err
	}
	if configPath := nugetUserConfigPath(packageManager); configPath != "" {
		result.status.Location = configPath
	}
	if !enabled {
		// A disabled source is not used for resolution.
		return result, nil
	}
	result.status.State, result.status.Host, result.status.RepoKey = classify(packageManager, sourceURL, serverDetails, nugetRepoKey)
	if result.status.State == StateConfigured {
		creds, hasCredentials := readNugetSourceCredentials(result.status.Location, dotnet.SourceName)
		result.status.Credentials = CredentialsAbsent
		if hasCredentials {
			result.status.Credentials = CredentialsPresent
			result.credentials = creds
		}
	}
	return result, nil
}

// nugetUserConfigPath is the per-user NuGet.Config the toolchain's `sources add` writes to:
// %APPDATA% on Windows, ~/.nuget for dotnet and ~/.config for nuget elsewhere. Each
// toolchain reads only its own file, so the other one is never consulted.
func nugetUserConfigPath(packageManager project.ProjectType) string {
	if coreutils.IsWindows() {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "NuGet", "NuGet.Config")
		}
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if packageManager == project.Nuget {
		return filepath.Join(home, ".config", "NuGet", "NuGet.Config")
	}
	return filepath.Join(home, ".nuget", "NuGet", "NuGet.Config")
}

type nugetConfigFile struct {
	Credentials struct {
		Sources []struct {
			XMLName xml.Name
			Entries []struct {
				Key   string `xml:"key,attr"`
				Value string `xml:"value,attr"`
			} `xml:"add"`
		} `xml:",any"`
	} `xml:"packageSourceCredentials"`
}

// readNugetSourceCredentials returns the credentials the NuGet.Config at configPath holds
// for sourceName. A password NuGet encrypted (Windows) counts as present but cannot be
// used by the probe.
func readNugetSourceCredentials(configPath, sourceName string) (creds storedCredentials, found bool) {
	if configPath == "" {
		return storedCredentials{}, false
	}
	content, exists, err := readOptionalFile(configPath)
	if err != nil || !exists {
		return storedCredentials{}, false
	}
	var parsed nugetConfigFile
	if unmarshalXML(content, &parsed) != nil {
		return storedCredentials{}, false
	}
	for _, source := range parsed.Credentials.Sources {
		if !strings.EqualFold(source.XMLName.Local, sourceName) {
			continue
		}
		for _, entry := range source.Entries {
			switch strings.ToLower(entry.Key) {
			case "username":
				creds.user = entry.Value
			case "cleartextpassword":
				creds.password = entry.Value
			case "password":
				found = found || entry.Value != ""
			}
		}
		return creds, found || creds.password != ""
	}
	return storedCredentials{}, false
}
