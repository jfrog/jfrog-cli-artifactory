package buildscan

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt bs [command options] <build name> <build number>"}

func GetDescription() string {
	return "Scan a published build-info with Xray."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "build name", Description: "Build name."},
		{Name: "build number", Description: "Build number."},
	}
}
