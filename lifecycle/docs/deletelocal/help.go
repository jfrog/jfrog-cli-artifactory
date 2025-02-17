package deletelocal

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbdell [command options] <release bundle name> <release bundle version>",
	"rbdell [command options] <release bundle name> <release bundle version> <environment>"}

func GetDescription() string {
	return "Delete all release bundle promotions to an environment or delete a release bundle locally altogether."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to delete locally."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to delete locally."},
		{Name: "environment", Description: "If provided, all promotions to this environment are deleted."},
	}
}
