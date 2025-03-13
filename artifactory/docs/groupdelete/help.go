package groupdelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt gdel <group name>"}

func GetDescription() string {
	return "Delete a users group."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "group name",
			Description: "Group name to be deleted.",
		},
	}
}
