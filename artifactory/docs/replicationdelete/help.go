package replicationdelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rpldel <repository key>"}

func GetDescription() string {
	return "Remove a replication repository from Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "repository key",
			Description: "The repository from which the replication will be deleted.",
		},
	}
}
