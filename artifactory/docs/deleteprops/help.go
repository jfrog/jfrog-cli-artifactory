package deleteprops

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Delete properties on existing files in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "files pattern",
			Description: "Properties of artifacts that match this pattern will be removed. " +
				"In the following format: <repository name>/<repository path>. You can use wildcards to specify multiple artifacts.",
		},
		{
			Name:        "properties list",
			Description: "List of comma-separated(,) properties, in the form of key1,key2,..., to be removed from the matching files.",
		},
	}
}
