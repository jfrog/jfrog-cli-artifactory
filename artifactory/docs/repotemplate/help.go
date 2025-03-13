package repotemplate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rpt <template path>"}

func GetDescription() string {
	return "Create a JSON template for repository creation or update."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file.",
		},
	}
}
