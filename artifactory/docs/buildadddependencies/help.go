package buildadddependencies

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bad [command options] <build name> <build number> <pattern>",
	"rt bad --spec=<File Spec path> [command options] <build name> <build number>",
}

func GetDescription() string {
	return "Adds dependencies from the local file-system to the build info."
}

func GetAIDescription() string {
	return `Attach files as build-info "dependencies" so the published build can record what inputs went into it. By default reads from the local filesystem; with --from-rt the pattern is evaluated against Artifactory.

When to use:
- Recording the version of a third-party binary checked into the workspace as a build input.
- Collecting checksums of inputs that did not flow through a package manager.
- Snapshotting Artifactory paths as inputs via --from-rt.

Prerequisites:
- A build name and number that will later be passed to jf rt build-publish.
- With --from-rt, a configured server with read permission on the source repo.

Common patterns:
  $ jf rt build-add-dependencies my-build 42 "deps/*.tgz"
  $ jf rt build-add-dependencies my-build 42 "vendor/lib.so" --regexp
  $ jf rt build-add-dependencies my-build 42 "tools-repo/cli/*" --from-rt

Gotchas:
- --regexp is not supported with --from-rt; mixing them errors out.
- Local mode does not upload anything; it only records checksums in the in-progress build-info.
- Files are recorded with build-info-relative paths; renaming sources between this command and build-publish breaks tooling.

Related: jf rt build-publish, jf rt build-add-git, jf rt build-collect-env, jf rt upload`
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
			Name: "pattern",
			Description: `Without the --from-rt option, this argument specifies the local file system 
path to dependencies which should be added to the build info.
You can specify multiple dependencies by using wildcards or a regular expression
as designated by the --regexp command option.
When the --from-rt option is added, this argument specifies a path in Artifactory
in the following format: <repository name>/<repository path>, from which the dependencies
should be collected and added to the build. You can use wildcards to specify multiple files.`,
		},
	}
}
