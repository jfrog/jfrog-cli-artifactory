package setprops

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{
	"rt sp [command options] <files pattern> <file properties>",
	"rt sp <file properties> --spec=<File Spec path> [command options]",
}

const EnvVar string = common.JfrogCliFailNoOp

func GetDescription() string {
	return "Set properties on existing files in Artifactory."
}

func GetAIDescription() string {
	return `Attach or overwrite properties (key=value pairs) on existing Artifactory artifacts. Properties drive search, retention, and promotion rules.

When to use:
- Tagging artifacts after promotion ("env=prod", "qa.passed=true").
- Marking artifacts for later cleanup ("deleteAfter=2026-01-01").
- Bulk-applying properties from a file spec.

Prerequisites:
- Configured server with annotate (set-props) permission on the matched artifacts.
- Pattern starts with repo name (or use --spec / --build / --bundle).

Common patterns:
  $ jf rt set-props "my-repo/com/example/*.jar" "qa.passed=true;env=staging"
  $ jf rt set-props --build=my-build/42 "stage=released"
  $ jf rt set-props --spec=props-spec.json "owner=team-a"

Gotchas:
- Properties are SEMICOLON-separated, not comma-separated.
- --repo-only restricts the operation to the repo descriptor, not artifacts inside it.
- Existing same-key values are overwritten; use delete-props first if you want a clean state.

Related: jf rt delete-props, jf rt search, jf rt upload (--target-props)`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "files pattern",
			Description: "Specifies the artifacts in Artifactory to apply properties to. Use <repository>/<path> format and wildcards (*, ?) to match multiple artifacts.",
		},
		{
			Name:        "file properties",
			Description: "List of semicolon-separated (;) key-value properties in the form of 'key1=value1;key2=value2;...'. These properties will be applied to matching artifacts.",
		},
	}
}
