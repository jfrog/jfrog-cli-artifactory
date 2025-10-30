package directdownload

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Download files using direct API respecting Artifactory's native resolution order and bypassing AQL."
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
