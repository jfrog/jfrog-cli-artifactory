package transferfiles

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt transfer-files [command options] <source-server-id> <target-server-id>"}

func GetDescription() string {
	return "Transfer files from one Artifactory to another."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source-server-id",
			Description: "Server ID of the Artifactory instance to transfer from.",
		},
		{
			Name:        "target-server-id",
			Description: "Server ID of the Artifactory instance to transfer to.",
		},
	}
}
