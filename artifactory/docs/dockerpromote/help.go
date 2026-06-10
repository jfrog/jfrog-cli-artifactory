package dockerpromote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt docker-promote <source docker image> <source repo> <target repo>"}

func GetDescription() string {
	return "Promotes a Docker image from one repository to another. Supported by local repositories only."
}

func GetAIDescription() string {
	return `Promote a Docker image between two LOCAL Docker repositories on the same Artifactory instance. Server-side operation; no docker daemon required.

When to use:
- Moving an image from docker-staging to docker-release after QA.
- Retagging an image during promotion via --target-tag / --target-docker-image.

Prerequisites:
- Both source and target repos must be LOCAL Docker repos on the same Artifactory; remote/virtual not supported.
- Configured server with read on source repo and deploy on target repo.

Common patterns:
  $ jf rt docker-promote my-app docker-staging docker-release
  $ jf rt docker-promote my-app docker-staging docker-release --source-tag=rc1 --target-tag=1.0.0
  $ jf rt docker-promote my-app docker-staging docker-release --target-docker-image=my-app-prod --copy=true

Gotchas:
- Default behavior MOVES; pass --copy=true to keep the image in the source repo.
- If --source-tag is omitted, ALL tags of the image are promoted.
- Will fail silently on remote/virtual repos; verify repo type with jf rt repo-template if uncertain.

Related: jf rt build-promote, jf rt copy, jf rt podman-push`
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
