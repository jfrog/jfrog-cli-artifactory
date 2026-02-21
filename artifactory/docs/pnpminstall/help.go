package pnpminstall

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt pnpmi [pnpm install args] [command options]"}

func GetDescription() string {
	return "Run pnpm install."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "pnpm install args",
			Description: "The pnpm install args to run pnpm install. " +
				"For example, --global.",
		},
	}
}
