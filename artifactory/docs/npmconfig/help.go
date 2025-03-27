package npmconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt npm-config [command options]"}

func GetDescription() string {
	return "Generate npm configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
