package buildclean

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bc <build name> <build number>",
}

func GetDescription() string {
	return "This command is used to clean (remove) build info collected locally."
}

func GetAIDescription() string {
	return `Clear locally collected build-info state for a given build name and number. Operates on the JFrog CLI's local cache only, not on data already published to Artifactory.

When to use:
- Restarting an in-progress build after a failed step that polluted the local build-info.
- Cleaning up before a fresh CI iteration when the working directory persists across builds.

Prerequisites:
- No server connection required; this is a local filesystem operation under JFROG_HOME.

Common patterns:
  $ jf rt build-clean my-build 42

Gotchas:
- Does NOT delete the published build-info from Artifactory. Use jf rt build-discard for that.
- Build name/number can come from JFROG_CLI_BUILD_NAME / JFROG_CLI_BUILD_NUMBER env vars.
- Safe to call repeatedly; if no local state exists, it is a no-op.

Related: jf rt build-publish, jf rt build-discard, jf rt build-collect-env`
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
