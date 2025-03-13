package dockerpush

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt docker-push <image tag> <target repo>"}

func GetDescription() string {
	return "Docker push."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "image tag",
			Description: "Docker image tag to push.",
		},
		{
			Name:        "target repo",
			Description: "Target repository in Artifactory.",
		},
	}
}
