package promote

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbp [command options] <release bundle name> <release bundle version> <environment>"}

func GetDescription() string {
	return "Promote a release bundle"
}

func GetAIDescription() string {
	return `Promote a Release Bundle v2 to a named environment (e.g. DEV, QA, PROD), creating a new immutable bundle version associated with that environment.

When to use:
- Advancing a release through promotion stages (DEV -> QA -> PROD).
- Including/excluding specific repos during promotion via --include-repos / --exclude-repos.
- Choosing the promotion strategy with --promotion-type.

Prerequisites:
- The bundle must be finalized (not in draft state).
- The target environment must be defined on the platform.
- A signing key configured.

Common patterns:
  $ jf release-bundle-promote my-bundle 1.0.0 QA --signing-key=my-key
  $ jf release-bundle-promote my-bundle 1.0.0 PROD --signing-key=my-key --sync --include-repos="libs-release;docker-prod"
  $ jf release-bundle-promote my-bundle 1.0.0 PROD --signing-key=my-key --exclude-repos="snapshots-*"

Gotchas:
- --include-repos / --exclude-repos use SEMICOLON separators, not commas.
- Without --sync the promotion is asynchronous.
- Project-scoped bundles need --project; otherwise default project is used.

Related: jf release-bundle-create, jf release-bundle-finalize, jf release-bundle-distribute`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to promote."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to promote."},
		{Name: "environment", Description: "Name of the target environment for the promotion."},
	}
}
