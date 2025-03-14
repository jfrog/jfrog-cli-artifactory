package transferplugininstall

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt transfer-plugin-install <server-id> [command options]"}

func GetDescription() string {
	return "Download and install the data-transfer user plugin on the primary node of Artifactory, which is running on this local machine."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "server-id",
			Description: "The ID of the source server, on which the plugin should be installed.",
		},
	}
}
