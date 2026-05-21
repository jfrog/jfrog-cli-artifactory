package podmanpull

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt podman-pull <image tag> <source repo>"}

func GetDescription() string {
	return "Podman pull."
}

func GetAIDescription() string {
	return `Run "podman pull" of an image from an Artifactory Docker repo, recording the dependency in build-info. Wraps the system podman binary.

When to use:
- Pulling a base image that should be tracked as a build dependency.
- Recording pulled image layers in a build via --build-name/--build-number.

Prerequisites:
- podman installed and on PATH.
- Configured server with read permission on the source repo.

Common patterns:
  $ jf rt podman-pull my-registry/my-app:1.0 docker-remote
  $ jf rt podman-pull my-registry/my-app:1.0 docker-remote --build-name=my-build --build-number=42

Gotchas:
- Source repo must be a Docker repo (local, remote, or virtual).
- --skip-login=true assumes podman is already authenticated to the registry.
- Image tag must include the Artifactory registry hostname.

Related: jf rt podman-push, jf rt docker-promote, jf rt build-publish`
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
