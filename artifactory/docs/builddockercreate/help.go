package builddockercreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt build-docker-create <target repo> --image-file=<Image file path>"}

func GetDescription() string {
	return "Add a published docker image to the build-info."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "target repo",
			Description: "The repository to which the image was pushed.",
		},
	}
}
