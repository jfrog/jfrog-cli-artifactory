package podmanpush

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt podman-push <image tag> <target repo>"}

func GetDescription() string {
	return "Podman push."
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
