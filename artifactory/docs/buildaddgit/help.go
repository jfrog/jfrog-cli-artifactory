package buildaddgit

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bag [command options] <build name> <build number> [Path To .git]",
}

func GetDescription() string {
	return `Collects the Git revision and URL from the local .git directory and adds it to the build-info.`
}

func GetAIDescription() string {
	return `Capture VCS metadata (commit SHA, branch, remote URL) from a local .git directory and attach it to the in-progress build-info. Optionally extract tracked issue IDs from commit messages using a YAML config.

When to use:
- Recording the exact commit a CI build came from for traceability.
- Linking builds to issues via --config (commit message regex pattern).

Prerequisites:
- A local .git directory; either run from the repo root or pass an explicit path.
- Build name+number that will be published with jf rt build-publish.
- For issue collection: a YAML config defining regex and tracker URL.

Common patterns:
  $ jf rt build-add-git my-build 42
  $ jf rt build-add-git my-build 42 /workspace/myrepo
  $ jf rt build-add-git my-build 42 --config=.jfrog/issues.yaml

Gotchas:
- Searches upward from the given path for a .git directory; symlinked repos may not be detected.
- A shallow clone (depth=1) may have no usable git log, causing empty issue extraction.
- Issues are extracted by regex; misconfigured patterns silently produce no matches.

Related: jf rt build-publish, jf rt build-collect-env, jf rt build-add-dependencies`
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
		{
			Name: "path to .git",
			Description: `Path to a directory containing the .git directory. If not specified, the .git directory is assumed to be in the current directory or in one of the parent directories.
It can also collect the list of tracked project issues (for example, issues stored in JIRA or other bug tracking systems) and add them to the build-info. 
The issues are collected by reading the git commit messages from the local git log.
Each commit message is matched against a pre-configured regular expression, which retrieves the issue ID and issue summary.
The information required for collecting the issues is retrieved from a yaml configuration file provided to the command.`,
		},
	}
}
