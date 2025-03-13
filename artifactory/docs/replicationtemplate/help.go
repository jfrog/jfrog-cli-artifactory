package replicationtemplate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rplt <template path>"}

func GetDescription() string {
	return "Create a JSON template for creating a replication repository."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file to be used for the replication creation.",
		},
	}
}
