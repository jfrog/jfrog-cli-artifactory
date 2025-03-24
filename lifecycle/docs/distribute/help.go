package distribute

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbd [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Distribute a release bundle."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to distribute."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to distribute."},
	}
}
