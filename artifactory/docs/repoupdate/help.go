package repoupdate

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var Usage = []string{"rt ru <template path>"}

func GetDescription() string {
	return "Update an existing repository configuration in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "template path",
			Description: "Specifies the local file system path for the template file to be used for the repository update. " +
				"The template can be created using the `" + coreutils.GetCliExecutableName() + " rt rpt` command.",
		},
	}
}
