package permissiontargettemplate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt ptt <template path>"}

func GetDescription() string {
	return "Create a JSON template for a permission target creation or replacement."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file.",
		},
	}
}
