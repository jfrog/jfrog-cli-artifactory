package buildcollectenv

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bce <build name> <build number>",
}

func GetDescription() string {
	return "Collect environment variables. Environment variables can be excluded using the build-publish command."
}

func GetAIDescription() string {
	return `Snapshot the current process environment variables and attach them to the in-progress build-info. Use this in CI to capture which env config the build ran under (CI vars, language versions, feature flags).

When to use:
- Recording CI variables (BUILD_ID, GIT_BRANCH, etc.) into build-info for later debugging.
- Before jf rt build-publish, to ensure the env snapshot is fresh.

Prerequisites:
- No server connection required; this writes to the local build-info cache.
- Build name+number must match what jf rt build-publish will use.

Common patterns:
  $ jf rt build-collect-env my-build 42

Gotchas:
- Captures EVERY env var visible to the process; use --env-exclude on build-publish to mask secrets before publishing.
- Default exclusion in build-publish drops *password*, *secret*, *token*, *key*, *auth*; widening that mask can leak credentials.
- Calling again for the same build overwrites the previous snapshot.

Related: jf rt build-publish, jf rt build-add-git, jf rt build-add-dependencies`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "Build name.",
		},
		{
			Name:        "build number",
			Description: "Build number.",
		},
	}
}
