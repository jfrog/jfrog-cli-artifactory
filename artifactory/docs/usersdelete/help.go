package usersdelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt udel <users list>", "rt udel --csv <users details file path>"}

func GetDescription() string {
	return "Delete users."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "users list",
			Description: "Comma-separated(,) list of usernames to delete in the form of user1,user2,....",
		},
		{
			Name:        "csv file path",
			Description: "Path to a CSV file containing user details for deletion.",
		},
	}
}
