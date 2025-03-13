package transferconfigmerge

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt transfer-config-merge [command options] <source-server-id> <target-server-id>"}

func GetDescription() string {
	return "Merge projects and repositories from a source Artifactory instance to a target Artifactory instance, if no conflicts are found"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source-server-id",
			Description: "The source server ID. The configuration will be exported from this server.",
		},
		{
			Name:        "target-server-id",
			Description: "The target server ID. The configuration will be imported to this server.",
		},
	}
}
