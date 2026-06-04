package annotate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rba [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Annotate a release bundle"
}

func GetAIDescription() string {
	return `Attach metadata to a Release Bundle v2: a tag, properties (key=value), or remove specific properties. Exactly one of --tag, --properties, --del-prop is required.

When to use:
- Tagging bundles ("approved-by=qa", "compliance=ok") for downstream filtering.
- Removing obsolete metadata with --del-prop.
- Adding a release tag with --tag.

Prerequisites:
- A configured platform server with annotate permission on the bundle.
- The bundle must exist (any state).

Common patterns:
  $ jf release-bundle-annotate my-bundle 1.0.0 --tag=approved
  $ jf release-bundle-annotate my-bundle 1.0.0 --properties="env=prod;owner=team-a"
  $ jf release-bundle-annotate my-bundle 1.0.0 --del-prop="stage,temp"

Gotchas:
- At least one of --tag, --properties, --del-prop must be set; missing returns an error.
- --properties uses SEMICOLON between key=value pairs.
- Defaults --project=default if no --project supplied.

Related: jf release-bundle-create, jf rt set-props`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to annotate."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to annotate."},
	}
}
