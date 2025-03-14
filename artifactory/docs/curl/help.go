package curl

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt curl [command options] <curl command>"}

func GetDescription() string {
	return "Execute a cUrl command, using the configured Artifactory details."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "curl command",
			Description: "cUrl command to run.",
		},
	}
}
