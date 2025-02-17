package create

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbc [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Create a release bundle from builds or from existing release bundles"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the newly created Release Bundle."},
		{Name: "release bundle version", Description: "Version of the newly created Release Bundle."},
	}
}
