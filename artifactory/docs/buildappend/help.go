package buildappend

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt ba <build name> <build number> <build name to append> <build number to append>",
}

func GetDescription() string {
	return "Append published build to the build info."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "The current build name.",
		},
		{
			Name:        "build number",
			Description: "The current build number.",
		},
		{
			Name:        "build name to append",
			Description: "The published build name to append to the current build.",
		},
		{
			Name:        "build number to append",
			Description: "The published build number to append to the current build.",
		},
	}
}
