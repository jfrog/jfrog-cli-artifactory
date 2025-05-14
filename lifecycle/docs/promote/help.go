package promote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbp [command options] <release bundle name> <release bundle version> <environment>"}

func GetDescription() string {
	return "Promote a release bundle"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to promote."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to promote."},
		{Name: "environment", Description: "Name of the target environment for the promotion."},
	}
}
