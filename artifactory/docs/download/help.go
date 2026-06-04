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

func GetAIDescription() string {
	return `Download files from an Artifactory repository to the local filesystem. Use when an agent needs to fetch artifacts, build outputs, or release-bundle contents into a working directory.

When to use:
- Pulling specific artifacts by AQL/wildcard pattern.
- Downloading every artifact attached to a build via --build-name/--build-number.
- Fetching the contents of a release bundle via --bundle=name/version.

Prerequisites:
- A configured server with read permission on the source repo (or anonymous if the repo allows it).
- Source pattern starts with the repo name (my-repo/path/...).
- For --bundle with a v2 bundle, supply --project if the bundle lives in a project.

Common patterns:
  $ jf rt download "my-repo/releases/app-1.0.tgz" ./dist/
  $ jf rt download "my-repo/com/example/*.jar" libs/ --flat=false
  $ jf rt download --build=my-build/42 ./artifacts/
  $ jf rt download --spec=download-spec.json --threads=8

Gotchas:
- A trailing slash on the target means "directory"; no slash means rename-to-this-file.
- --sync-deletes deletes files locally to match the remote; combine with --dry-run first or pass --quiet.
- Transitive download (across remote-repo virtuals) requires JFROG_CLI_TRANSITIVE_DOWNLOAD=true.
- Default --recursive=true; set --recursive=false for shallow matches only.

Related: jf rt upload, jf rt direct-download, jf rt search, jf rt copy`
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
