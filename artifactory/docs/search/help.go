package search

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{
	"rt s [command options] <search pattern>",
	"rt s --spec=<File Spec path> [command options]",
}

const EnvVar string = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Search files in Artifactory."
}

func GetAIDescription() string {
	return `Search Artifactory for artifacts matching a pattern, build, or release bundle. Prints results as JSON (with checksums, sizes, properties) for downstream tooling, or just a count via --count.

When to use:
- Listing artifacts in a repo path before deleting/promoting them.
- Inspecting what is attached to a build (--build=name/number) or release bundle.
- Filtering by properties: --props "key=value;k2=v2".

Prerequisites:
- A configured server with read permission on the searched repo(s).
- Pattern begins with the repo name (my-repo/path/...).

Common patterns:
  $ jf rt search "my-repo/com/example/*.jar" --include=name,size,sha256
  $ jf rt search --build=my-build/42
  $ jf rt search "my-repo/*" --props "qa.tested=true" --count

Gotchas:
- Empty result is success by default; set JFROG_CLI_FAIL_NO_OP=true to make empty results error.
- Output is JSON to stdout; large result sets benefit from --limit and --sort-by.
- --transitive only works against virtual repos pointing at remote repos.

Related: jf rt download, jf rt delete, jf rt set-props`
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
