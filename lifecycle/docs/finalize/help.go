package finalize

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbf [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Finalize a draft release bundle."
}

func GetAIDescription() string {
	return `Finalize a DRAFT Release Bundle v2, signing it and making it immutable. After finalize, the bundle can be promoted and distributed.

When to use:
- Locking a draft bundle once content assembly is complete.
- Step before jf release-bundle-promote / jf release-bundle-distribute.

Prerequisites:
- The bundle exists and is in draft state.
- A signing key configured on the platform (--signing-key).
- Configured platform server.

Common patterns:
  $ jf release-bundle-finalize my-bundle 1.0.0 --signing-key=my-key
  $ jf release-bundle-finalize my-bundle 1.0.0 --signing-key=my-key --sync --project=my-proj

Gotchas:
- One-way operation: a finalized bundle cannot return to draft.
- Without --sync the finalize is asynchronous; subsequent commands may fail while it is still in progress.

Related: jf release-bundle-create, jf release-bundle-update, jf release-bundle-promote`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to finalize."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to finalize."},
	}
}
