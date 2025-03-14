package permissiontargetupdate

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var Usage = []string{"rt ptu <template path>"}

func GetDescription() string {
	return "Update a permission target in the JFrog Platform."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "template path",
			Description: "Specifies the local file system path for the template file to be used for the permission target update. " +
				"The template can be created using the \"" + coreutils.GetCliExecutableName() + " rt ptu\" command.",
		},
	}
}
