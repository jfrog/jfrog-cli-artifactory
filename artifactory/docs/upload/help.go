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

func GetAIDescription() string {
	return `Upload one or more files from the local filesystem to an Artifactory repository. Use this when an agent needs to publish artifacts, attach files to a build, or sync a local directory tree into a repo.

When to use:
- Publishing a built artifact (tgz, jar, zip, binary) to a generic or package-typed repo.
- Attaching files as build dependencies/artifacts via --build-name/--build-number.
- Mirroring a local folder to Artifactory with --sync-deletes.

Prerequisites:
- A configured server (jf c add or jf login) with deploy permission on the target repo.
- Target path must include the repository name as the first segment (my-repo/path/...).
- For --regexp, the first matching group in the source pattern must be parenthesized.

Common patterns:
  $ jf rt upload "dist/app.tgz" my-repo/releases/
  $ jf rt upload "build/*.jar" libs-release-local/com/example/app/1.0/ --build-name=app --build-number=42
  $ jf rt upload --spec=upload-spec.json --detailed-summary

Gotchas:
- A trailing slash on the target means "folder"; no trailing slash means rename-to-this-filename. Easy to misuse.
- --sync-deletes can delete artifacts in Artifactory; pair with --dry-run first, or pass --quiet to suppress the confirmation prompt.
- Wildcards behave differently than --regexp; mixing them is invalid. Pick one mode.
- --flat=true (default for some patterns) flattens directory structure; set --flat=false to preserve hierarchy.

Related: jf rt download, jf rt copy, jf rt search, jf rt set-props`
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
