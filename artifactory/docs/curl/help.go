package curl

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt curl [command options] <curl command>"}

func GetDescription() string {
	return "Execute a cUrl command, using the configured Artifactory details."
}

func GetAIDescription() string {
	return `Invoke any Artifactory REST API endpoint using curl-style arguments, with auth and base URL injected automatically from the configured server. Escape hatch for endpoints not covered by dedicated commands.

When to use:
- Calling administrative or experimental APIs not wrapped by the CLI (e.g. /api/system/configuration).
- Scripting custom JSON requests against /api/repositories, /api/security, etc.

Prerequisites:
- A configured server (jf c add or jf login); the access token / credentials are added to the request automatically.
- The path passed should be relative to the Artifactory base URL (start with /api/...).

Common patterns:
  $ jf rt curl /api/system/ping
  $ jf rt curl -XPOST /api/repositories/libs-local -H "Content-Type: application/json" -d @repo.json
  $ jf rt curl -XGET /api/build/my-build/42 --server-id=my-prod

Gotchas:
- Flag parsing is OFF (SkipFlagParsing); everything after "rt curl" is passed verbatim to curl.
- Use --server-id to pick a specific server when several are configured.
- The base URL is the Artifactory one; for platform or lifecycle APIs, prefer their dedicated paths.

Related: jf rt ping, jf c show, jf rt repo-create`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "curl command",
			Description: "cUrl command to run.",
		},
	}
}
