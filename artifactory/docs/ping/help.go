package ping

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt ping [command options]"}

func GetDescription() string {
	return "Send applicative ping to Artifactory."
}

func GetAIDescription() string {
	return `Send an application-level ping to Artifactory and print the JSON response. Use to verify connectivity and authentication before running heavier commands.

When to use:
- Smoke-testing a new server config or token in CI before downloads/uploads.
- Diagnosing whether a 401/403 is auth (returns error) vs. network (returns timeout).

Prerequisites:
- A configured server (jf c add or jf login) or explicit --url/--user/--access-token flags.

Common patterns:
  $ jf rt ping
  $ jf rt ping --server-id=my-prod

Gotchas:
- Anonymous ping works against an unauthenticated server; a successful ping does not imply repo access.
- Output is indented JSON on stdout; "OK" status means the API is reachable.

Related: jf c show, jf rt curl /api/system/health`
}

func GetArguments() []components.Argument {
	return nil
}
