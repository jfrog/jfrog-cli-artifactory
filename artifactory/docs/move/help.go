package move

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt mv [command options] <source pattern> <target pattern>",
	"rt mv --spec=<File Spec path> [command options]"}

var EnvVar = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Move files between Artifactory paths."
}

func GetAIDescription() string {
	return `Move artifacts between paths inside Artifactory. Server-side operation that deletes the source after copying. Use when an agent needs to relocate artifacts permanently (e.g. promote a snapshot and drop the original).

When to use:
- Relocating artifacts from one repo to another without keeping a duplicate.
- Cleaning up staging repos by moving accepted builds out.

Prerequisites:
- A configured server with delete permission on source and deploy permission on target.
- Both patterns begin with the repo name (my-repo/path/...).

Common patterns:
  $ jf rt move "staging-repo/com/example/*-1.0.jar" release-repo/com/example/
  $ jf rt move --build=my-build/42 release-repo/
  $ jf rt move --spec=move-spec.json --dry-run

Gotchas:
- The source is deleted after copy; --dry-run first is strongly recommended.
- Same trailing-slash rule as copy/upload: slash = folder, no slash = rename target.
- Failures partway through can leave artifacts in a mixed state; check the summary.

Related: jf rt copy, jf rt delete, jf rt upload, jf rt build-promote`
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
