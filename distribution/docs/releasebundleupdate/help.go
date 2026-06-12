package releasebundleupdate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbu [command options] <release bundle name> <release bundle version> <pattern>",
	"ds rbu --spec=<File Spec path> [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Updates an existing unsigned release bundle v1 version."
}

func GetAIDescription() string {
	return `Modify the artifact set of an UNSIGNED Distribution v1 release bundle version. Once a bundle is signed, it is immutable; updates require deleting and recreating it.

When to use:
- Iterating on bundle contents during development before signing.
- Correcting a mistake in a freshly created v1 bundle.

Prerequisites:
- A configured server with Distribution endpoints.
- The bundle must exist and be UNSIGNED.
- Read permission on source repos.

Common patterns:
  $ jf ds release-bundle-update my-bundle 1.0.0 "my-repo/com/example/*.jar"
  $ jf ds release-bundle-update my-bundle 1.0.0 --spec=updated-spec.json
  $ jf ds release-bundle-update my-bundle 1.0.0 "my-repo/path/" --sign

Gotchas:
- --detailed-summary requires --sign.
- Updating a signed bundle errors; delete with jf ds release-bundle-delete and recreate.
- Passing --sign after edit promotes the bundle to signed in one step.

Related: jf ds release-bundle-create, jf ds release-bundle-sign, jf ds release-bundle-delete`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "The name of the release bundle."},
		{Name: "release bundle version", Description: "The release bundle version."},
		{Name: "pattern", Description: `Specifies the source path in Artifactory, from which the artifacts should be 
										bundled,\n\t\tin the following format: <repository name>/<repository path>. 
										You can use wildcards to specify multiple artifacts.`},
	}
}
