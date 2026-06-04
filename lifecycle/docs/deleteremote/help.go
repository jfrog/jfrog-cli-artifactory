package deleteremote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbdelr [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Delete a release bundle remotely."
}

func GetAIDescription() string {
	return `Remove a distributed Release Bundle v2 from edge nodes matching the supplied distribution rules. Does not delete the source bundle.

When to use:
- Recalling a distributed bundle from specific edges (e.g. after a critical bug is found).
- Cleaning up old distributions on edge sites.

Prerequisites:
- A configured platform server with delete permission on the bundle.
- Distribution rules via --dist-rules or --site/--city/--country-code.

Common patterns:
  $ jf release-bundle-delete-remote my-bundle 1.0.0 --site="edge-eu" --quiet
  $ jf release-bundle-delete-remote my-bundle 1.0.0 --dist-rules=./rules.json --sync --max-wait-minutes=30

Gotchas:
- Does NOT affect the local copy; use jf release-bundle-delete-local for that.
- Interactive confirmation by default; --quiet for CI.
- --dist-rules conflicts with --site/--city/--country-code.

Related: jf release-bundle-delete-local, jf release-bundle-distribute`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to delete remotely."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to delete remotely."},
	}
}
