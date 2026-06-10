package update

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbu [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Update an existing draft release bundle. The --add flag is mandatory to specify the type of operation."
}

func GetAIDescription() string {
	return `Add more sources (builds or release bundles) to an existing DRAFT Release Bundle v2. Only draft bundles can be updated; finalized bundles are immutable.

When to use:
- Iteratively assembling a draft bundle across multiple CI stages before finalize.
- Adding late-arriving builds to a draft bundle prior to release.

Prerequisites:
- The release bundle must exist in DRAFT state (created with --draft).
- Configured platform server.
- --add is mandatory along with at least one source method (--spec or --source-type-builds/--source-type-release-bundles).

Common patterns:
  $ jf release-bundle-update my-bundle 1.0.0 --add --spec=more-builds.json
  $ jf release-bundle-update my-bundle 1.0.0 --add --source-type-builds=./builds.json --sync

Gotchas:
- Errors out if no source method is supplied; --add alone is not enough.
- Cannot update non-draft bundles; finalize is one-way.
- Without --sync the operation is asynchronous.

Related: jf release-bundle-create, jf release-bundle-finalize, jf release-bundle-promote`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to update."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to update."},
	}
}
