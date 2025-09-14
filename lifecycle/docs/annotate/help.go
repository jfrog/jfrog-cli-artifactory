package annotate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rba [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Annotate a release bundle"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to annotate."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to annotate."},
	}
}
