package update

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbu [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Update an existing draft release bundle by adding sources"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to update."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to update."},
	}
}
