package cocoapodsconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt cocoapods-config [command options]"}

func GetDescription() string {
	return "Generate cocoapods build configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
