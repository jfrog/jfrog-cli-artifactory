package releasebundledistribute

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbd [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Distribute a release bundle v1."
}

func GetAIDescription() string {
	return `Push a signed Distribution v1 release bundle to one or more edge nodes (target Artifactories) according to distribution rules.

When to use:
- Final step of v1 release pipeline: push signed bundle to edges.
- Geo-distribution via site/city/country-code matching rules.

Prerequisites:
- The bundle must be signed (jf ds release-bundle-sign).
- A configured server with Distribution endpoints.
- Distribution rules defined either via --dist-rules or --site/--city/--country-code flags.

Common patterns:
  $ jf ds release-bundle-distribute my-bundle 1.0.0 --site="edge-eu" --sync
  $ jf ds release-bundle-distribute my-bundle 1.0.0 --dist-rules=./rules.json --max-wait-minutes=30
  $ jf ds release-bundle-distribute my-bundle 1.0.0 --create-repo=true --country-code="US,DE"

Gotchas:
- --dist-rules cannot be combined with --site/--city/--country-code; pick one approach.
- Without --sync, the command returns once the request is accepted, not when distribution completes.
- --create-repo auto-creates missing target repos on edge nodes.

Related: jf ds release-bundle-create, jf ds release-bundle-sign, jf ds release-bundle-delete`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Release bundle name."},
		{Name: "release bundle version", Description: "Release bundle version."},
	}
}
