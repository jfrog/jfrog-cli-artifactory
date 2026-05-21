package search

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbs <option>"}

func GetDescription() string {
	return "This command is used to search release-bundle groups(names) and versions APIs."
}

func GetAIDescription() string {
	return `Search Release Bundle v2 catalog by names (groups) or by versions of a given name. The first positional argument selects which API to call.

When to use:
- Listing all release bundle names on the platform.
- Finding all versions of a specific release bundle.
- Driving downstream automation from bundle metadata.

Prerequisites:
- A configured platform server with read access to the lifecycle service.
- For "versions": the release bundle name must be provided.

Common patterns:
  $ jf release-bundle-search names
  $ jf release-bundle-search versions my-bundle
  $ jf release-bundle-search versions my-bundle --filter-by=tag=approved --order-by=created --order-asc=false --format=json

Gotchas:
- "names" takes no extra args; "versions" requires exactly the bundle name.
- Flag applicability differs between names and versions; consult the lifecycle REST API docs.
- --project filters versions to a specific project.

Related: jf release-bundle-create, jf release-bundle-promote`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "option",
			Description: "Available options are : names, versions.\n" +
				"\t\tExample: jf rbs names \n" +
				"\t\tExample: jf rbs versions release-bundle-name\n" +
				"\t\tAll Available flags are not applicable with all options. For flags applicable to specific option, please refer to https://jfrog.com/help/r/jfrog-rest-apis/release-lifecycle-management",
		},
	}
}
