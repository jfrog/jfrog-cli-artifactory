package permissiontargetdelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt ptdel <permission target name>"}

func GetDescription() string {
	return "Permanently delete a permission target."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "permission target name",
			Description: "Specifies the permission target that should be removed.",
		},
	}
}
