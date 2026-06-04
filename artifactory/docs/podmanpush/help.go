package podmanpush

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt podman-push <image tag> <target repo>"}

func GetDescription() string {
	return "Podman push."
}

func GetAIDescription() string {
	return `Run "podman push" of a local image to an Artifactory Docker repo, recording the operation as build-info. Wraps the system podman binary, so podman must be installed and configured.

When to use:
- Pushing OCI images built with podman to Artifactory.
- Recording image layers in a build via --build-name/--build-number.

Prerequisites:
- podman installed and reachable in PATH.
- Configured server with deploy permission on the target repo.
- The local image to push (built or pulled prior).

Common patterns:
  $ jf rt podman-push my-registry/my-app:1.0 docker-local
  $ jf rt podman-push my-registry/my-app:1.0 docker-local --build-name=my-build --build-number=42
  $ jf rt podman-push my-registry/my-app:1.0 docker-local --threads=4 --detailed-summary

Gotchas:
- Target repo must be a Docker repo; pushing to a generic repo fails silently from the user's perspective.
- --skip-login=true is required if you have already authenticated podman to the registry outside the CLI.
- Image tag must include the Artifactory registry hostname (my-registry/my-app:1.0).

Related: jf rt podman-pull, jf rt docker-promote, jf rt build-publish`
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
