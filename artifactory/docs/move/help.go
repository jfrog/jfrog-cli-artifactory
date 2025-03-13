package move

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli/docs/common"
)

var Usage = []string{"rt mv [command options] <source pattern> <target pattern>",
	"rt mv --spec=<File Spec path> [command options]"}

var EnvVar = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Move files between Artifactory paths."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source pattern",
			Description: "Specifies the source path in Artifactory, from which the artifacts should be moved, in the format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.",
		},
		{
			Name:        "target pattern",
			Description: "Specifies the target path in Artifactory, to which the artifacts should be moved, in the format: <repository name>/<repository path>. If the pattern ends with a slash, the target path is assumed to be a folder. If there is no terminal slash, the target path is assumed to be a file to which the moved file should be renamed. Placeholders in the form of {1}, {2} can be used to replace corresponding tokens from the source path.",
		},
	}
}
