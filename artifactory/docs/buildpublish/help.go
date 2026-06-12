package buildpublish

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt bp [command options] <build name> <build number>"}

func GetDescription() string {
	return "Publish build info."
}

func GetAIDescription() string {
	return `Publish a build-info record to Artifactory, aggregating everything previously associated with the build (uploaded artifacts, dependencies, git data, env). Call this at the end of a CI pipeline so subsequent commands (promote, scan, discard) have something to operate on.

When to use:
- Final step of a CI build after upload/build-add-* calls.
- Associating a build with a project via --project.
- Capturing environment variables (--env-include/--env-exclude) and git info (--git=true).

Prerequisites:
- A configured server with deploy permission to the build-info repo (artifactory-build-info by default).
- Build name + number must match what was used in earlier rt commands during the build.

Common patterns:
  $ jf rt build-publish my-build 42
  $ jf rt build-publish my-build 42 --project=my-proj --build-url=https://ci/build/42
  $ jf rt build-publish my-build 42 --detailed-summary --env-exclude="*token*;*secret*"

Gotchas:
- Build name/number can also be sourced from JFROG_CLI_BUILD_NAME / JFROG_CLI_BUILD_NUMBER (env then args overwrite).
- Default --env-exclude masks common secret patterns; widening it can leak credentials into the build-info.
- After publish, subsequent rt commands using the same build/number will create a separate revision.

Related: jf rt build-collect-env, jf rt build-add-git, jf rt build-promote, jf rt build-discard`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "build name", Description: "Build name."},
		{Name: "build number", Description: "Build number."},
	}
}
