package twinedocs

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"twine <twine arguments> [command options]"}

func GetDescription() string {
	return "Runs twine "
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "twine commands",
			Description: "Arguments and options for the twine command.",
		},
	}
}
