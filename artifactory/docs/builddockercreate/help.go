package builddockercreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt build-docker-create <target repo> --image-file=<Image file path>"}

func GetDescription() string {
	return "Add a published docker image to the build-info."
}

func GetAIDescription() string {
	return `Record a previously-pushed Docker image (and all its layers) as an artifact of the current build-info. Use this when the image was pushed by a tool other than the JFrog CLI (e.g. native docker/buildah CLI, kaniko) and you still want the build-info to reflect it.

When to use:
- Image was pushed via "docker push" or kaniko outside of jf rt commands.
- Linking the image manifest to a build for later promote/scan operations.

Prerequisites:
- The image must already exist in the target repo with the manifest reachable.
- A local file containing the image-name-and-digest in the format: <image>@sha256:<digest>.
- Configured server with read on the target repo.

Common patterns:
  $ jf rt build-docker-create docker-local --image-file=./image-digest.txt --build-name=my-build --build-number=42

Gotchas:
- --image-file is required; the file must contain exactly one line like "my-registry/my-app@sha256:abc...".
- Reads the manifest from Artifactory to enumerate layers; missing manifest causes failure.
- Does NOT push the image; it only adds metadata to the in-progress build-info.

Related: jf rt build-publish, jf rt docker-promote, jf rt podman-push`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "target repo",
			Description: "The repository to which the image was pushed.",
		},
	}
}
