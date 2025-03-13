package transferconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt transfer-config [command options] <source-server-id> <target-server-id>"}

func GetDescription() string {
	return `Copy full Artifactory configuration from source Artifactory server to target Artifactory server.`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source-server-id",
			Description: "The source server ID. The configuration will be exported from this server.",
		},
		{
			Name:        "target-server-id",
			Description: "The target server ID. The configuration will be imported to this server.\n[Warning] This action will wipe all Artifactory content in this target server.",
		},
	}
}
