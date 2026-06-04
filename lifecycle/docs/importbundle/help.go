package importbundle

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbi [command options] <path to archive>"}

func GetDescription() string {
	return "Import a local release bundle archive to Artifactory"
}

func GetAIDescription() string {
	return `Upload a previously-exported Release Bundle v2 archive (.zip) from the local filesystem to the platform. Mirror operation of jf release-bundle-export.

When to use:
- Restoring a bundle from a backup or airgapped transfer.
- Importing a bundle produced on another platform.

Prerequisites:
- A configured platform server with import permission.
- A valid release bundle archive on disk (the artifact produced by release-bundle-export).

Common patterns:
  $ jf release-bundle-import ./bundles/my-bundle-1.0.0.zip

Gotchas:
- Expects exactly one positional arg (path to the archive).
- Bundle name/version are read from the archive metadata; re-importing the same archive may conflict with an existing version.
- No size guard; very large archives may time out on slow links.

Related: jf release-bundle-export, jf release-bundle-create`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "path to archive", Description: "Path to the release bundle archive on the filesystem"},
	}
}
