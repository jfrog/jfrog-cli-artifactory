package delete

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Delete files from Artifactory."
}

func GetAIDescription() string {
	return `Delete artifacts from Artifactory. Use when an agent needs to clean up obsolete builds, remove broken uploads, or implement retention.

When to use:
- Removing old snapshots after a release.
- Deleting every artifact attached to a build via --build=name/number.
- Bulk removal via a file spec or wildcard pattern.

Prerequisites:
- A configured server with delete permission on the target repo.
- Pattern starts with the repo name.

Common patterns:
  $ jf rt delete "snapshot-repo/com/example/*-SNAPSHOT/"
  $ jf rt delete --build=my-build/42 --quiet
  $ jf rt delete --spec=delete-spec.json --dry-run

Gotchas:
- Interactive confirmation prompt by default; pass --quiet to bypass (required in CI).
- --dry-run prints what would be deleted without doing it; always use first on broad patterns.
- Deletes are immediate and irreversible unless the repo has trash-can/retention enabled.
- --recursive defaults to true; combine with precise patterns.

Related: jf rt search, jf rt delete-props, jf rt build-discard`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "delete pattern",
			Description: "Specifies the source path in Artifactory, from which the artifacts should be deleted, " +
				"in the following format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.",
		},
	}
}
