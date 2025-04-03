package delete

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Delete files from Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "delete pattern",
			Description: "Specifies the source path in Artifactory, from which the artifacts should be deleted, " +
				"in the following format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.",
		},
	}
}
