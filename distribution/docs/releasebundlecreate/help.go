package releasebundlecreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbc [command options] <release bundle name> <release bundle version> <pattern>",
	"ds rbc --spec=<File Spec path> [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Create a release bundle v1."
}

func GetAIDescription() string {
	return `Create a Distribution v1 release bundle that captures a set of Artifactory artifacts at a specific point in time. Bundle is unsigned until --sign is passed (or jf ds rbs is run separately). v1 is legacy; prefer the lifecycle (rbv2) commands for new pipelines.

When to use:
- Packaging artifacts for distribution to edge nodes via classic Distribution.
- Producing signed, immutable artifact sets for compliance.

Prerequisites:
- A configured server pointing to JFrog Distribution (DS) endpoints.
- Read permission on the source repos.
- For --sign: a GPG signing key configured in Distribution.

Common patterns:
  $ jf ds release-bundle-create my-bundle 1.0.0 "my-repo/com/example/*.jar"
  $ jf ds release-bundle-create my-bundle 1.0.0 --spec=bundle-spec.json --sign
  $ jf ds release-bundle-create my-bundle 1.0.0 "my-repo/path/" --release-notes-path=./RELEASE.md --release-notes-syntax=markdown

Gotchas:
- --detailed-summary requires --sign; otherwise the command rejects it.
- --release-notes-syntax auto-detects from .md/.markdown extension; explicit flag overrides.
- v1 release bundles are deprecated in favor of rbv2 (jf release-bundle-create); confirm which one your platform supports.

Related: jf ds release-bundle-sign, jf ds release-bundle-distribute, jf release-bundle-create`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "The name of the release bundle."},
		{Name: "release bundle version", Description: "The release bundle version."},
		{Name: "pattern", Description: `Specifies the source path in Artifactory, from which the artifacts should be 
					bundled,\n\t\tin the following format: <repository name>/<repository path>. You can use wildcards 
					to specify multiple artifacts.`},
	}
}
