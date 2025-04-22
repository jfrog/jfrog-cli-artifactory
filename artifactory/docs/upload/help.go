package upload

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt u [command options] <source pattern> <target pattern>",
	"rt u --spec=<File Spec path> [command options]"}

var EnvVar = []string{common.JfrogCliMinChecksumDeploySizeKb, common.JfrogCliFailNoOp, common.JfrogCliUploadEmptyArchive}

func GetDescription() string {
	return "Upload files from local file system to Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "source pattern",
			Description: `Specifies the local file system path to artifacts which should be uploaded to Artifactory.
You can specify multiple artifacts by using wildcards or a regular expression as designated by the --regexp command option.
If you have specified that you are using regular expressions, then the first one used in the argument must be enclosed in parenthesis.`,
		},
		{
			Name: "target pattern",
			Description: `Specifies the target path in Artifactory in the following format: <repository name>/<repository path>.
If the target path ends with a slash, the path is assumed to be a folder. For example, if you specify the target as "repo-name/a/b/",
then "b" is assumed to be a folder in Artifactory into which files should be uploaded. If there is no terminal slash, the target path
is assumed to be a file to which the uploaded file should be renamed. For example, if you specify the target as "repo-name/a/b",
the uploaded file is renamed to "b" in Artifactory.
For flexibility in specifying the upload path, you can include placeholders in the form of {1}, {2} which are replaced by corresponding
tokens in the source path that are enclosed in parenthesis.`,
		},
	}
}
