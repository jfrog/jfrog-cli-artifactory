package buildcollectenv

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bce <build name> <build number>",
}

func GetDescription() string {
	return "Collect environment variables. Environment variables can be excluded using the build-publish command."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "Build name.",
		},
		{
			Name:        "build number",
			Description: "Build number.",
		},
	}
}
