package builddiscard

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"rt bdi [command options] <build name>",
}

func GetDescription() string {
	return "Discard builds by setting retention parameters."
}

func GetAIDescription() string {
	return `Discard old builds in Artifactory using retention rules (max number of builds, max age in days, exclusions). Optionally delete the underlying artifacts too. Use this to implement build retention policy from a pipeline.

When to use:
- Applying a "keep last N builds" rule from CI.
- Aging out builds older than X days.
- Combined with --delete-artifacts=true to also reclaim repo storage.

Prerequisites:
- Configured server with delete permission on the build-info repo (and on artifact repos if --delete-artifacts is used).

Common patterns:
  $ jf rt build-discard my-build --max-builds=20
  $ jf rt build-discard my-build --max-days=30 --delete-artifacts=true
  $ jf rt build-discard my-build --exclude-builds=42,43 --max-builds=10 --async=true

Gotchas:
- --delete-artifacts removes the actual files from repos; irreversible unless trash-can is enabled.
- --max-builds and --max-days can be combined; either condition triggers discard.
- --async=true returns immediately; check Artifactory logs for completion.

Related: jf rt build-publish, jf rt build-clean, jf rt delete`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "build name",
			Description: "Build name.",
		},
	}
}
