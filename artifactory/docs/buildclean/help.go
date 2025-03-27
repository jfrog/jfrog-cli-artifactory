package buildclean

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bc <build name> <build number>",
}

func GetDescription() string {
	return "This command is used to clean (remove) build info collected locally."
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
