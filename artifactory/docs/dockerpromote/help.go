package dockerpromote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt docker-promote <source docker image> <source repo> <target repo>"}

func GetDescription() string {
	return "Promotes a Docker image from one repository to another. Supported by local repositories only."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "source docker image",
			Description: "The docker image name to promote.",
		},
		{
			Name:        "source repo",
			Description: "Source repository in Artifactory.",
		},
		{
			Name:        "target repo",
			Description: "Target repository in Artifactory.",
		},
	}
}
