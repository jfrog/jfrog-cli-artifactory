package copy

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt cp [command options] <source pattern> <target pattern>",
	"rt cp --spec=<File Spec path> [command options]"}

const EnvVar string = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Copy files between Artifactory paths."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source pattern",
			Description: `Specifies the source path in Artifactory, from which the artifacts should be copied, in the following format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.`,
		},
		{
			Name: "target pattern",
			Description: `Specifies the target path in Artifactory, to which the artifacts should be copied, in the following format: <repository name>/<repository path>.
If the pattern ends with a slash, the target path is assumed to be a folder. For example, if you specify the target as "repo-name/a/b/",
then "b" is assumed to be a folder in Artifactory into which files should be copied.
If there is no terminal slash, the target path is assumed to be a file to which the copied file should be renamed.
For example, if you specify the target as "repo-name/a/b", the copied file is renamed to "b" in Artifactory.
For flexibility in specifying the upload path, you can include placeholders in the form of {1}, {2} which are replaced by corresponding
tokens in the source path that are enclosed in parenthesis.`,
		},
	}
}
