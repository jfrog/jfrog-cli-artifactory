package nuget

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"nuget restore [options] [command options]",
	"nuget push <package> [options] [command options]",
	"nuget pack [options] [command options]",
	"nuget <nuget args> [command options]",
}

func GetDescription() string {
	return "Run NuGet with Artifactory integration. Set JFROG_RUN_NATIVE=true to enable native tool wrapping with build-info collection."
}

func GetAIDescription() string {
	return `Run a NuGet command via nuget.exe with optional Artifactory integration.

When JFROG_RUN_NATIVE=true is set, jf wraps the native nuget.exe and collects build-info automatically.

Subcommands that collect build-info:
  restore / install / update   Dependency collection: resolves packages and records them as build-info dependencies.
  push                         Artifact collection: pushes .nupkg/.snupkg to Artifactory and records as build-info artifacts.
  pack                         Artifact collection: records produced .nupkg/.snupkg as build-info artifacts.

All other nuget subcommands are passed through to nuget.exe unchanged (no build-info collected).

Without JFROG_RUN_NATIVE=true, this command uses the legacy config-file approach (requires 'jf nuget-config' to be run first).

Common patterns:
  $ JFROG_RUN_NATIVE=true jf nuget restore --repo-resolve my-nuget-virtual --build-name myBuild --build-number 1
  $ JFROG_RUN_NATIVE=true jf nuget push MyLib.1.0.0.nupkg --repo my-nuget-local --build-name myBuild --build-number 1
  $ JFROG_RUN_NATIVE=true jf nuget pack --build-name myBuild --build-number 1
  $ jf rt bp myBuild 1

Gotchas:
  - .slnx solution files are not supported by nuget.exe; use 'jf dotnet restore' for .slnx projects.
  - Without JFROG_RUN_NATIVE=true, 'jf nuget-config' must be run first to configure repositories.

Related: jf dotnet, jf nuget-config, jf rt build-publish`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "nuget command",
			Description: "The nuget.exe subcommand to run (e.g. restore, push, pack, install, update). " +
				"When JFROG_RUN_NATIVE=true, restore/install/update collect dependency build-info, " +
				"push and pack collect artifact build-info. All other subcommands are passed through unchanged.",
		},
	}
}
