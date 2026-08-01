package cli

func GetDescription() string {
	return "Agent Package Manager (APM) commands with JFrog Artifactory authentication."
}

func GetAIDescription() string {
	return `Run any apm command against JFrog Artifactory-backed registries, with credentials
injected automatically - no apm config set or manual token handling required.

Build-info commands (dedicated subcommands, listed under COMMANDS below):
  jf agent apm install   Install dependencies from apm.yml / apm.lock.yaml.
  jf agent apm publish   Publish a package to an agentpackages repository.
  jf agent apm update    Refresh dependencies to their latest matching refs.
These three collect and can record build-info (--build-name/--build-number).

Every other apm command also works here, with the same authenticated registry access but
no build-info collection - just run it as "jf agent apm <command>", e.g.:
  jf agent apm lock                 Resolve dependencies and write apm.lock.yaml only.
  jf agent apm deps why <pkg>       Show why a dependency is present (direct/transitive).
  jf agent apm outdated             Show outdated locked dependencies.
  jf agent apm audit                 Scan installed packages / validate lockfile integrity.
  jf agent apm doctor                Diagnose environment problems (git, network, auth).
  jf agent apm view <pkg>            View package metadata or list remote versions.
  jf agent apm marketplace ...       Manage marketplaces for discovery and governance.
  jf agent apm mcp ...               Discover, inspect, and install MCP servers.
Run "apm --help" to see the full list of commands apm itself supports - all of them are
reachable this way. "jf agent apm <command> --help" shows that command's own apm-native help.

Prerequisites:
- apm CLI installed and in PATH.
- Registry configured via jf setup agent-apm (persistent) or apm.yml's registries: block.

Environment:
- Credentials injected via APM_REGISTRY_TOKEN_<NAME>, APM_REGISTRY_USER_<NAME>, APM_REGISTRY_PASS_<NAME>.
- Registry configuration sourced from apm.yml's registries: block or ~/.apm/config.json (set by jf setup agent-apm).

Related: jf setup agent-apm`
}
