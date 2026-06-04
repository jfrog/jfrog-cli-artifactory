package deletelocal

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbdell [command options] <release bundle name> <release bundle version>",
	"rbdell [command options] <release bundle name> <release bundle version> <environment>"}

func GetDescription() string {
	return "Delete all release bundle promotions to an environment or delete a release bundle locally altogether."
}

func GetAIDescription() string {
	return `Delete a Release Bundle v2 from the local platform. With two args, deletes the bundle version entirely; with a third (environment) arg, only the promotions to that environment are removed.

When to use:
- Removing an obsolete bundle version on the source platform.
- Undoing a promotion by deleting the bundle's association with a single environment.

Prerequisites:
- A configured platform server with delete permission on the bundle.
- Interactive confirmation unless --quiet.

Common patterns:
  $ jf release-bundle-delete-local my-bundle 1.0.0 --quiet
  $ jf release-bundle-delete-local my-bundle 1.0.0 QA
  $ jf release-bundle-delete-local my-bundle 1.0.0 --project=my-proj --sync

Gotchas:
- Does not affect bundles already distributed to edge nodes; use jf release-bundle-delete-remote for that.
- Without --sync the delete is asynchronous.
- Two-arg form deletes everything for the version; the env-scoped form is non-destructive elsewhere.

Related: jf release-bundle-delete-remote, jf release-bundle-promote`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to delete locally."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to delete locally."},
		{Name: "environment", Description: "If provided, all promotions to this environment are deleted."},
	}
}
