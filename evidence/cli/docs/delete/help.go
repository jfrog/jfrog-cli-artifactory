package delete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return ` Delete evidence associated with a specified subject by evidence name. `
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
