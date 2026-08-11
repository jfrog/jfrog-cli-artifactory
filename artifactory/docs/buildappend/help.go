package buildappend

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt ba <build name> <build number> <build name to append> <build number to append>",
}

func GetDescription() string {
	return "Append published build to the build info."
}

func GetAIDescription() string {
	return `Reference an already-published build as part of the current in-progress build-info. Useful for composite builds where one pipeline aggregates several upstream builds.

When to use:
- Aggregating microservice builds into a single release build-info.
- Linking a deploy pipeline to the upstream artifact pipeline.

Prerequisites:
- The target build (build-name-to-append/build-number-to-append) must already be published in Artifactory.
- Configured server with read permission on the build-info repo.

Common patterns:
  $ jf rt build-append release-build 1.0 service-a-build 42
  $ jf rt build-append release-build 1.0 service-b-build 17

Gotchas:
- Order of arguments: current build first, then the appended build.
- Both builds must live on the same Artifactory instance.
- The appended build's artifacts are not duplicated; only the reference is added.

Related: jf rt build-publish, jf rt build-promote

QA:
Q: How do I append build 'build1' with number '1' to 'master-build' with number '10'?
A: jf rt ba master-build 10 build1 1

Q: Could you tell me the command to incorporate 'build3' with the number '3' into 'appBuild' with the number '30'?
A: jf rt ba appBuild 30 build3 3

Q: How do I add 'build5' build number '5' to my 'simpleBuild' with number '50'?
A: jf rt ba simpleBuild 50 build5 5
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "The current build name.",
		},
		{
			Name:        "build number",
			Description: "The current build number.",
		},
		{
			Name:        "build name to append",
			Description: "The published build name to append to the current build.",
		},
		{
			Name:        "build number to append",
			Description: "The published build number to append to the current build.",
		},
	}
}
