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

Related: jf rt delete-props, jf rt search, jf rt upload (--target-props)

QA:
Q: How to set properties on .jar files in "MavenCentral"?
A: jf rt sp "MavenCentral/*.jar" "key=value"

Q: How can I set properties on files in a specific Artifactory server ID for files matching the pattern 'my-repo/my-path/*' and properties 'key1=value1;key2=value2'?
A: jf rt sp 'my-repo/my-path/*' 'key1=value1;key2=value2' --server-id=myServerID

Q: How can I set properties on files using a file spec named 'myFileSpec.json' for properties 'key1=value1;key2=value2'?
A: jf rt sp 'key1=value1;key2=value2' --spec=myFileSpec.json

Q: How can I set properties on files using variables in the file spec named 'myFileSpec.json' for properties 'key1=value1;key2=value2' and spec variables 'key1=value1;key2=value2'?
A: jf rt sp 'key1=value1;key2=value2' --spec=myFileSpec.json --spec-vars='key1=value1;key2=value2'

Q: How can I set properties 'key1=value1;key2=value2' on files that have specific properties 'key1=value1;key2=value2' for files matching the pattern 'my-repo/my-path/*'?
A: jf rt sp 'my-repo/my-path/*' 'key1=value1;key2=value2' --props=key1=value1;key2=value2

Q: How can I set properties 'key1=value1;key2=value2' on files that do not have specific properties 'key1=value1;key2=value2' for files matching the pattern 'my-repo/my-path/*'?
A: jf rt sp 'my-repo/my-path/*' 'key1=value1;key2=value2' --exclude-props=key1=value1;key2=value2
`
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
