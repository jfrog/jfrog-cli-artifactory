package releasebundleupdate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbu [command options] <release bundle name> <release bundle version> <pattern>",
	"ds rbu --spec=<File Spec path> [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Updates an existing unsigned release bundle v1 version."
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
