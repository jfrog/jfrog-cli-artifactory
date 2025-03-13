package nuget

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt nuget <nuget args> [command options]"}

func GetDescription() string {
	return "Run NuGet."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "nuget command",
			Description: "The nuget command to run. For example, restore.",
		},
	}
}
