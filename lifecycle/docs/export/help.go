package export

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbe <release bundle name> <release bundle version> [target pattern]"}

func GetDescription() string {
	return "Triggers the Export process and downloads the Release Bundle archive"
}

func GetAIDescription() string {
	return `Trigger export of a Release Bundle v2 and download the resulting archive to the local filesystem. Supports path remapping during export.

When to use:
- Producing an offline copy of a bundle for airgapped distribution.
- Backing up a bundle outside the JFrog platform.

Prerequisites:
- A configured platform server with read on the bundle.
- Writable local target path (defaults to current directory).

Common patterns:
  $ jf release-bundle-export my-bundle 1.0.0 ./bundles/
  $ jf release-bundle-export my-bundle 1.0.0 --project=my-proj
  $ jf release-bundle-export my-bundle 1.0.0 ./out/ --mapping-pattern="(.*)/staging/(.*)" --mapping-target="$1/release/$2"

Gotchas:
- Trailing slash on target = directory; no slash = rename target file.
- Export is server-side; large bundles can take minutes. Use --threads and --split-count to tune local download.
- --skip-checksum=true disables local integrity verification.

Related: jf release-bundle-import, jf release-bundle-distribute, jf rt download`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to export."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to export."},
		{Name: "target pattern", Description: "The third argument is optional and specifies the local file system target path.\n\t\tIf the target path ends with a slash, the path is assumed to be a directory.\n\t\tFor example, if you specify the target as \"repo-name/a/b/\", then \"b\" is assumed to be a directory into which files should be downloaded.\n\t\tIf there is no terminal slash, the target path is assumed to be a file to which the downloaded file should be renamed.\n\t\tFor example, if you specify the target as \"a/b\", the downloaded file is renamed to \"b\"."},
	}
}
