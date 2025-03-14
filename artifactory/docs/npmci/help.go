package npmci

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt npmci [npm ci args] [command options]"}

func GetDescription() string {
	return "Run npm ci."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "npm ci args",
			Description: "The npm ci args to run npm ci.",
		},
	}
}
