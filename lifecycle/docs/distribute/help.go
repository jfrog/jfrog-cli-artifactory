package distribute

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbd [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Distribute a release bundle."
}

func GetAIDescription() string {
	return `Distribute a Release Bundle v2 to one or more edge nodes (target Artifactories) according to distribution rules. Optionally rewrite artifact paths during distribution with --mapping-pattern / --mapping-target.

When to use:
- Pushing a finalized/promoted bundle to geo-distributed edges.
- Re-pathing artifacts during distribution (e.g. strip "staging-" prefix on the edge).

Prerequisites:
- The bundle must be finalized.
- A configured platform server.
- Distribution rules via --dist-rules or --site/--city/--country-code.

Common patterns:
  $ jf release-bundle-distribute my-bundle 1.0.0 --site="edge-us" --sync
  $ jf release-bundle-distribute my-bundle 1.0.0 --dist-rules=./rules.json --max-wait-minutes=60
  $ jf release-bundle-distribute my-bundle 1.0.0 --site="edge-eu" --mapping-pattern="(.*)/staging/(.*)" --mapping-target="$1/prod/$2"

Gotchas:
- --mapping-pattern and --mapping-target must be provided together; either alone errors out.
- --dist-rules conflicts with --site/--city/--country-code.
- --create-repo auto-creates missing target repos on edge nodes.

Related: jf release-bundle-promote, jf release-bundle-delete-remote, jf release-bundle-export`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to distribute."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to distribute."},
	}
}
