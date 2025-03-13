package terraformdocs

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"terraform <terraform arguments> [command options]"}

func GetDescription() string {
	return "Runs terraform "
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "terraform commands",
			Description: "Specifies the Terraform command to execute, along with any necessary arguments and options.",
		},
	}
}
