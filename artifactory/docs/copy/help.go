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

func GetAIDescription() string {
	return `Copy artifacts between paths inside Artifactory without moving them. The operation is server-side, so no bytes flow through the client. Use when an agent needs to promote, mirror, or duplicate artifacts across repos.

When to use:
- Promoting an artifact from a snapshot to a release repo while keeping the original.
- Mirroring a release bundle's contents to another repo.
- Duplicating artifacts attached to a build (--build=name/number).

Prerequisites:
- A configured server with read on the source repo and deploy on the target repo.
- Both source and target patterns must begin with the repo name.

Common patterns:
  $ jf rt copy "snapshot-repo/com/example/*.jar" release-repo/com/example/
  $ jf rt copy --build=my-build/42 staging-repo/
  $ jf rt copy --spec=copy-spec.json --dry-run

Gotchas:
- Trailing slash on target = folder; no trailing slash = rename. Same semantics as upload/move.
- --flat=true flattens directory structure on the target side.
- Always smoke-test with --dry-run before bulk copies; there is no undo.
- Cross-repo-type copy may fail (e.g. Maven layout to a generic repo retains paths but loses metadata).

Related: jf rt move, jf rt upload, jf rt search, jf rt build-promote`
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
