package deleteprops

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Delete properties on existing files in Artifactory."
}

func GetAIDescription() string {
	return `Remove specific properties from existing Artifactory artifacts. The artifacts themselves are not affected.

When to use:
- Clearing temporary tags after a promotion completes.
- Removing per-environment metadata when an artifact moves between stages.
- Cleaning up legacy property keys.

Prerequisites:
- Configured server with annotate permission on the matched artifacts.
- Pattern starts with repo name (or use --spec / --build / --bundle).

Common patterns:
  $ jf rt delete-props "my-repo/com/example/*.jar" "stage,owner"
  $ jf rt delete-props --build=my-build/42 "qa.passed"

Gotchas:
- Properties argument is COMMA-separated keys here (unlike set-props which uses semicolons with values).
- Only the listed keys are removed; other properties on the artifact remain.
- --repo-only narrows to repo-descriptor properties.

Related: jf rt set-props, jf rt search, jf rt delete`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "files pattern",
			Description: "Properties of artifacts that match this pattern will be removed. " +
				"In the following format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.",
		},
		{
			Name:        "properties list",
			Description: "List of comma-separated(,) properties, in the form of key1,key2,..., to be removed from the matching files.",
		},
	}
}
