package download

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt dl [command options] <source pattern> [target pattern]",
	"rt dl --spec=<File Spec path> [command options]"}

var EnvVar = []string{common.JfrogCliTransitiveDownload, common.JfrogCliFailNoOp}

func GetDescription() string {
	return "Download files from Artifactory to local file system."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source pattern",
			Description: "Specifies the source path in Artifactory, from which the artifacts should be downloaded, in the format: <repository name>/<repository path>. Wildcards can be used to specify multiple artifacts.",
		},
		{
			Name: "target pattern",
			Description: `Optional argument specifying the local file system target path.
If the target path ends with a slash, it is assumed to be a directory.
If there is no terminal slash, the target path is assumed to be a file.
Placeholders in the form of {1}, {2} can be used, replaced by corresponding tokens in the source path enclosed in parentheses.`,
		},
	}
}
