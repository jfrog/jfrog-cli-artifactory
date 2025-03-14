package replicationcreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rplc <template path>"}

func GetDescription() string {
	return "Create a new replication in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file to be used to create a replication. The template can be created using the “jfrog rt rplt” command.",
		},
	}
}
