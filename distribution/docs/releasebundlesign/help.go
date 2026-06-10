package releasebundlesign

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbs [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Sign a release bundle v1."
}

func GetAIDescription() string {
	return `Cryptographically sign an existing UNSIGNED Distribution v1 release bundle, making it immutable and eligible for distribution to edge nodes.

When to use:
- Finalizing a v1 bundle after creation/update for release.
- Re-running signing after rotating the GPG key (requires bundle recreation in practice).

Prerequisites:
- A configured server with Distribution endpoints.
- A signing (GPG) key configured in Distribution.
- The bundle must exist and be unsigned.

Common patterns:
  $ jf ds release-bundle-sign my-bundle 1.0.0
  $ jf ds release-bundle-sign my-bundle 1.0.0 --passphrase=$KEY_PASSPHRASE
  $ jf ds release-bundle-sign my-bundle 1.0.0 --repo=dist-storage-repo --detailed-summary

Gotchas:
- Signed bundles cannot be modified; only deleted and recreated.
- --passphrase must match the configured GPG key passphrase, if any.
- --repo specifies an optional storing repository for the bundle content.

Related: jf ds release-bundle-create, jf ds release-bundle-distribute, jf ds release-bundle-delete`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Release bundle name."},
		{Name: "release bundle version", Description: "Release bundle version."},
	}
}
