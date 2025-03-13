package pipconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt pip-config"}

func GetDescription() string {
	return "Generate pip build configuration."
}

func GetArguments() []components.Argument {
	return nil
}
