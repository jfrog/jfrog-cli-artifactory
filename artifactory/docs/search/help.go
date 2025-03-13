package search

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli/docs/common"
)

var Usage = []string{
	"rt s [command options] <search pattern>",
	"rt s --spec=<File Spec path> [command options]",
}

const EnvVar string = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Search files in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "search pattern",
			Description: "Specifies the search path in Artifactory, in the following format: <repository name>/<repository path>. " +
				"You can use wildcards to specify multiple artifacts.",
		},
	}
}
