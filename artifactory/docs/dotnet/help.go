package dotnet

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"dotnet restore [options] [command options]",
	"dotnet build [options] [command options]",
	"dotnet nuget push <package> [options] [command options]",
	"dotnet pack [options] [command options]",
	"dotnet <dotnet sub-command> [command options]",
}

func GetDescription() string {
	return "Run .NET CLI with Artifactory integration. Set JFROG_RUN_NATIVE=true to enable native tool wrapping with build-info collection."
}

func GetAIDescription() string {
	return `Run a .NET CLI command with optional Artifactory integration.

When JFROG_RUN_NATIVE=true is set, jf wraps the native dotnet CLI and collects build-info automatically.

Subcommands that collect build-info:
  restore / build / add        Dependency collection: resolves packages and records them as build-info dependencies.
  nuget push                   Artifact collection: pushes .nupkg/.snupkg to Artifactory and records as build-info artifacts.
  pack                         Artifact collection: records produced .nupkg/.snupkg as build-info artifacts.

All other dotnet subcommands (test, run, publish, etc.) are passed through to the dotnet CLI unchanged (no build-info collected).

Without JFROG_RUN_NATIVE=true, this command uses the legacy config-file approach (requires 'jf dotnet-config' to be run first).

Common patterns:
  $ JFROG_RUN_NATIVE=true jf dotnet restore --repo-resolve my-nuget-virtual --build-name myBuild --build-number 1
  $ JFROG_RUN_NATIVE=true jf dotnet build --build-name myBuild --build-number 1
  $ JFROG_RUN_NATIVE=true jf dotnet nuget push MyLib.1.0.0.nupkg --repo my-nuget-local --build-name myBuild --build-number 1
  $ JFROG_RUN_NATIVE=true jf dotnet pack --build-name myBuild --build-number 1
  $ jf rt bp myBuild 1

Gotchas:
  - .slnx solution files are supported only via 'jf dotnet restore', not 'jf nuget restore' (nuget.exe does not support .slnx).
  - Without JFROG_RUN_NATIVE=true, 'jf dotnet-config' must be run first to configure repositories.

Related: jf nuget, jf dotnet-config, jf rt build-publish`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "dotnet sub-command",
			Description: "The dotnet CLI subcommand to run (e.g. restore, build, pack, nuget push). " +
				"When JFROG_RUN_NATIVE=true, restore/build/add collect dependency build-info, " +
				"nuget push and pack collect artifact build-info. All other subcommands are passed through unchanged.",
		},
	}
}
