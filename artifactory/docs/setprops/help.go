package setprops

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli/docs/common"
)

var Usage = []string{
	"rt sp [command options] <files pattern> <file properties>",
	"rt sp <file properties> --spec=<File Spec path> [command options]",
}

const EnvVar string = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Set properties on existing files in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "files pattern",
			Description: "Specifies the artifacts in Artifactory to apply properties to. Use <repository>/<path> format and wildcards (*, ?) to match multiple artifacts.",
		},
		{
			Name:        "file properties",
			Description: "List of semicolon-separated (;) key-value properties in the form of 'key1=value1;key2=value2;...'. These properties will be applied to matching artifacts.",
		},
	}
}
