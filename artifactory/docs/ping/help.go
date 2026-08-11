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

Related: jf c show, jf rt curl /api/system/health

QA:
Q: How can I verify the accessibility of my default Artifactory server?
A: jf rt ping

Q: How can I ping the Artifactory server through the specified URL 'https://my-rt-server.com/artifactory'?
A: jf rt ping --url=https://my-rt-server.com/artifactory

Q: How can I check connection to Artifactory server with ID 'rt-server-2' without verifying TLS certificates?
A: jf rt ping --server-id=rt-server-2 --insecure-tls
Warning: Use --insecure-tls only for controlled troubleshooting; it disables TLS certificate validation.
`
}

func GetArguments() []components.Argument {
	return nil
}
