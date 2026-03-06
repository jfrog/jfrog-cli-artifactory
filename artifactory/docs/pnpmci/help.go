package pnpmci

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt pnpm ci [pnpm ci args] [command options]"}

func GetDescription() string {
	return "Run pnpm ci."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "pnpm ci args",
			Description: "The pnpm ci args to run pnpm ci.",
		},
	}
}
