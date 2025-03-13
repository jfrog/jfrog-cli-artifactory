package podmanpull

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt podman-pull <image tag> <source repo>"}

func GetDescription() string {
	return "Podman pull."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "image tag",
			Description: "Docker image tag to pull.",
		},
		{
			Name:        "source repo",
			Description: "Source repository in Artifactory.",
		},
	}
}
