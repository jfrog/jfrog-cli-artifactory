package dotnet

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt dotnet <dotnet sub-command> [command options]"}

func GetDescription() string {
	return "Run .NET Core CLI."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "dotnet sub-command",
			Description: "Arguments and options for the dotnet command.",
		},
	}
}
