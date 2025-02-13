package releasebundledelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"ds rbdel [command options] <release bundle name> <release bundle version>"}

func GetDescription() string {
	return "Delete a release bundle v1."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Release bundle name."},
		{Name: "release bundle version", Description: "Release bundle version."},
	}
}
