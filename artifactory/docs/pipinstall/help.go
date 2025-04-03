package pipinstall

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt pipi <pip sub-command>"}

func GetDescription() string {
	return "Run pip install."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "pip sub-command",
			Description: "Arguments and options for the pip command.",
		},
	}
}
