package groupcreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt gc <group name>"}

func GetDescription() string {
	return "Create a new user group."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "group name",
			Description: "The name of the new group.",
		},
	}
}
