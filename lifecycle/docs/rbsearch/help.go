package search

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbs <option>"}

func GetDescription() string {
	return "This command is used to search release-bundle groups(names) and versions APIs."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "option",
			Description: "Available options are : names, versions.\n" +
				"\t\tExample: jf rbs names \n" +
				"\t\tExample: jf rbs versions release-bundle-name\n" +
				"\t\tAll Available flags are not applicable with all options. For flags applicable to specific option, please refer to https://jfrog.com/help/r/jfrog-rest-apis/release-lifecycle-management",
		},
	}
}
