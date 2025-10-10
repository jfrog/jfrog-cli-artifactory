package search

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbs <subcommand>"}

func GetDescription() string {
	return "release-bundle-search"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "subcommand name",
			Description: "Available Subcommands are : names, versions. Available flags are not applicable on all subcommands.\n For flags applicable to specific subcommands, please refer to https://jfrog.com/help/r/jfrog-rest-apis/release-lifecycle-management",
		},
		{
			Name:        "format",
			Description: "Default output format is table. Another table format is json format.",
		},
	}
}
