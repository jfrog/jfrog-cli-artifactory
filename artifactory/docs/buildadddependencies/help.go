package buildadddependencies

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bad [command options] <build name> <build number> <pattern>",
	"rt bad --spec=<File Spec path> [command options] <build name> <build number>",
}

func GetDescription() string {
	return "Adds dependencies from the local file-system to the build info."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "Build name.",
		},
		{
			Name:        "build number",
			Description: "Build number.",
		},
		{
			Name: "pattern",
			Description: `Without the --from-rt option, this argument specifies the local file system 
path to dependencies which should be added to the build info.
You can specify multiple dependencies by using wildcards or a regular expression
as designated by the --regexp command option.
When the --from-rt option is added, this argument specifies a path in Artifactory
in the following format: <repository name>/<repository path>, from which the dependencies
should be collected and added to the build. You can use wildcards to specify multiple files.`,
		},
	}
}
