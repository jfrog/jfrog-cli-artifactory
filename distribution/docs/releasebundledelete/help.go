package releasebundledelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbdel [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Delete a release bundle v1."
}

func GetAIDescription() string {
	return `Delete a Distribution v1 release bundle from the source Distribution service, and optionally from edge nodes (--delete-from-dist).

When to use:
- Cleanup of obsolete or compromised release bundles.
- Removing a signed bundle to recreate it with different content.

Prerequisites:
- A configured server with Distribution endpoints.
- Admin/delete permission on the bundle.

Common patterns:
  $ jf ds release-bundle-delete my-bundle 1.0.0 --quiet
  $ jf ds release-bundle-delete my-bundle 1.0.0 --delete-from-dist --sync --max-wait-minutes=30
  $ jf ds release-bundle-delete my-bundle 1.0.0 --dist-rules=./rules.json

Gotchas:
- --delete-from-dist removes the bundle from edges too; without it only the source copy is removed.
- --dist-rules conflicts with --site/--city/--country-code.
- Interactive confirmation by default; pass --quiet for CI.

Related: jf ds release-bundle-create, jf ds release-bundle-distribute`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Release bundle name."},
		{Name: "release bundle version", Description: "Release bundle version."},
	}
}
