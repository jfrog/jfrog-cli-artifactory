package create

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbc [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Create a release bundle from builds or from existing release bundles"
}

func GetAIDescription() string {
	return `Create a Release Bundle v2 from one or more published builds or from existing Release Bundles. Modern lifecycle equivalent of Distribution v1 bundles; use this for new pipelines.

When to use:
- Packaging a set of published builds into a versioned, signable Release Bundle.
- Aggregating several existing Release Bundles into a higher-level bundle.
- Working with multiple sources of mixed types via --source-type-builds and --source-type-release-bundles (Artifactory 7.114.0+).

Prerequisites:
- A configured platform server (--url is the JFrog Platform URL).
- A signing key configured on the platform (--signing-key).
- For build sources: builds must already be published.
- For project-scoped bundles: --project=my-proj.

Common patterns:
  $ jf release-bundle-create my-bundle 1.0.0 --signing-key=my-key --builds=./builds-spec.json
  $ jf release-bundle-create my-bundle 1.0.0 --signing-key=my-key --release-bundles=./rbs-spec.json
  $ jf release-bundle-create my-bundle 1.0.0 --signing-key=my-key --spec=create-spec.json --sync

Gotchas:
- Exactly one source method (--spec, --builds, --release-bundles) must be supplied for the regular path; multi-source flags (--source-type-builds, --source-type-release-bundles) require platform >= 7.114.0.
- Without --sync the command returns immediately; the bundle creation continues asynchronously.
- --draft creates the bundle in draft state; finalize it later with jf release-bundle-finalize.

Related: jf release-bundle-update, jf release-bundle-promote, jf release-bundle-finalize, jf release-bundle-distribute`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the newly created Release Bundle."},
		{Name: "release bundle version", Description: "Version of the newly created Release Bundle."},
	}
}
