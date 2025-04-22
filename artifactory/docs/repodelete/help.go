package repodelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rdel <repository pattern>"}

func GetDescription() string {
	return "Permanently delete repositories with all of their content from Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "repository pattern",
			Description: "Specifies the repositories that should be removed. You can use wildcards to specify multiple repositories.",
		},
	}
}
