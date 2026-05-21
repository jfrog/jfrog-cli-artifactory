package buildpromote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt bpr [command options] <build name> <build number> <target repository>"}

func GetDescription() string {
	return "This command is used to promote build in Artifactory."
}

func GetAIDescription() string {
	return `Promote all artifacts of a published build to a target repository, optionally applying properties, changing build status, or including dependencies. Use this when a CI build has been approved and its outputs need to move from staging to release.

When to use:
- Promoting after QA passes: artifacts move from staging to release-local.
- Tagging a build with status="Released" and properties for downstream filtering.
- Including dependencies (--include-dependencies) when moving the entire closure.

Prerequisites:
- The build must already be published (jf rt build-publish).
- Configured server with deploy permission on the target repo and read on the source.

Common patterns:
  $ jf rt build-promote my-build 42 release-local
  $ jf rt build-promote my-build 42 release-local --status=Released --comment="QA approved"
  $ jf rt build-promote my-build 42 release-local --copy=true --props "stage=prod"

Gotchas:
- Default behavior moves (not copies) artifacts; pass --copy=true to retain in the source repo.
- --source-repo restricts promotion to artifacts that came from one staging repo; otherwise all are promoted.
- --fail-fast=true (default) aborts on the first error, leaving a partial promotion. Set --fail-fast=false for best-effort.
- The build itself is not duplicated; only the artifacts.

Related: jf rt build-publish, jf rt copy, jf rt move, jf release-bundle-promote`
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
			Name:        "target repository",
			Description: "Build promotion target repository.",
		},
	}
}
