package pnpmconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt pnpm-config [command options]"}

func GetDescription() string {
	return "Generate pnpm configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
