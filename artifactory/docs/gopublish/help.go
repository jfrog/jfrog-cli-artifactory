package gopublish

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt gp [command options] <project version>"}

func GetDescription() string {
	return "Publish a Go package and/or its dependencies to Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "project version",
			Description: "Specifies the version of the Go package to be published.",
		},
	}
}
