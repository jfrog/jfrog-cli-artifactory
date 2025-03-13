package buildpromote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt bpr [command options] <build name> <build number> <target repository>"}

func GetDescription() string {
	return "This command is used to promote build in Artifactory."
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
		{
			Name:        "target repository",
			Description: "Build promotion target repository.",
		},
	}
}
