package directdownload

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Download files using direct API respecting Artifactory's native resolution order and bypassing AQL."
}

func GetAIDescription() string {
	return `Download artifacts by issuing a direct GET (not AQL), which honors a virtual repo's configured resolution order. Use this when the same artifact path exists in several backing repos and you must get the one Artifactory itself would resolve.

When to use:
- Downloading from a virtual repo where order-of-precedence matters.
- Avoiding AQL-permission requirements (AQL needs read access on all underlying repos).
- Reproducing exactly what a docker/maven/npm client would pull.

Prerequisites:
- A configured server with read permission on the virtual (or its backing) repo.
- Source pattern starts with the repo name.

Common patterns:
  $ jf rt direct-download "virtual-repo/path/to/artifact.zip" ./downloads/
  $ jf rt direct-download --spec=download-spec.json
  $ jf rt direct-download "virtual-repo/release/app.jar" --build-name=my-build --build-number=42

Gotchas:
- Wildcards are limited compared to jf rt download (AQL-based).
- Per-path GETs may be slower than AQL-driven batch download for large patterns.
- Does not enumerate via search; you must know the exact paths or use --spec.

Related: jf rt download, jf rt search, jf rt upload`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source pattern",
			Description: "The source pattern in Artifactory that describes the artifacts to be downloaded.",
		},
		{
			Name:        "target pattern",
			Description: "The local file system path to which the artifacts should be downloaded. Default: .",
		},
	}
}

var Usage = []string{
	"jf rt ddl [command options] <source pattern> [target pattern]",
	"",
	"Examples:",
	"",
	"1. Download a single artifact from a virtual repository:",
	"   jf rt ddl 'virtual-repo/path/to/artifact.zip' './downloads/'",
	"",
	"2. Download using file spec:",
	"   jf rt ddl --spec=download-spec.json",
	"",
	"3. Download with build info collection:",
	"   jf rt ddl 'virtual-repo/release/app.jar' --build-name=myBuild --build-number=1",
	"",
	"Note: This command uses direct API calls instead of AQL, ensuring that artifacts",
	"are resolved according to the virtual repository's configured resolution order.",
	"This is particularly important when the same artifact exists in multiple",
	"repositories within a virtual repository.",
}
