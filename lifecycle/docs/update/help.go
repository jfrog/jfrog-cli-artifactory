package update

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbu [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Update an existing draft release bundle. The --add flag is mandatory to specify the type of operation."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to update."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to update."},
	}
}
