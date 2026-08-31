package cli

func GetDescription() string {
	return "Agent Package Manager (APM) commands with JFrog Artifactory authentication."
}

func GetAIDescription() string {
	return `Run apm against Artifactory-backed registries with credentials injected automatically. Dedicated subcommands install and publish also collect build-info when --build-name and --build-number are set; every other apm command (including update) is forwarded with the same authenticated registry access but no build-info collection.

When to use:
- Running apm install / publish with Artifactory auth and optional build-info.
- Running any other apm command (update, lock, outdated, audit, doctor, view, marketplace, mcp, ...) through jf for authenticated registry access.

Prerequisites:
- apm CLI installed and on PATH.
- Registry configured via 'jf setup apm' or an apm.yml registries: block.
- A configured JFrog Platform server (jf c add / jf login), or pass --server-id.

Common patterns:
  $ jf agent apm install --build-name=my-build --build-number=1
  $ jf agent apm publish --package my-org/my-package --build-name=my-build --build-number=1
  $ jf agent apm update --yes
  $ jf agent apm lock
  $ jf agent apm outdated

Gotchas:
- Build-info is collected only by install and publish, and only when both --build-name and --build-number are provided; publish it afterwards with 'jf rt build-publish'.
- 'jf agent apm update' is a plain passthrough - it refreshes apm.yml/apm.lock.yaml like the native CLI always did, but does not collect build-info.
- 'jf agent apm <command> --help' shows that command's own help; 'apm --help' lists every native apm command reachable this way.

Related: jf setup apm, jf agent apm install, jf agent apm publish, jf rt build-publish`
}
