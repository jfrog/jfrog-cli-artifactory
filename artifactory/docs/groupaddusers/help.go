package groupaddusers

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt gau <group name> <users list>"}

func GetDescription() string {
	return "Add a list of users to a group."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "group name",
			Description: "The name of the group.",
		},
		{
			Name:        "users list",
			Description: "Specifies the usernames to add to the specified group. The list should be comma-separated (e.g., user1,user2,...).",
		},
	}
}
