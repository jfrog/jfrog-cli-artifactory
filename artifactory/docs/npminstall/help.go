package npminstall

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt npmi [npm install args] [command options]"}

func GetDescription() string {
	return "Run npm install."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "npm install args",
			Description: "The npm install args to run npm install. " +
				"For example, --global.",
		},
	}
}
