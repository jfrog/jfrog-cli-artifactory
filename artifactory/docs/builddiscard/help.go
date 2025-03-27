package builddiscard

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bdi [command options] <build name>",
}

func GetDescription() string {
	return "Discard builds by setting retention parameters."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "Build name.",
		},
	}
}
