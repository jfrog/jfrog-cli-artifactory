package deleteremote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbdelr [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Delete a release bundle remotely."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to delete remotely."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to delete remotely."},
	}
}
