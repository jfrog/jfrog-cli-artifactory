package update

func GetDescription() string {
	return "Refresh APM dependencies to their latest matching refs, with build-info collection."
}

func GetAIDescription() string {
	return `Update packages in apm.yml to their latest versions matching declared constraints, refresh apm.lock.yaml, and optionally record a build-info of the resolved dependencies.

When to use:
- Keeping agent package dependencies current within declared version constraints.
- Re-resolving when new versions are published to the registry.
- Capturing build-info for an update by passing --build-name and --build-number.

Prerequisites:
- apm CLI installed and on PATH.
- An apm.yml with dependencies declared, and an existing apm.lock.yaml (e.g. from 'jf agent apm install').
- Read permission on the source Artifactory agentpackages repository.
- Registry configured via 'jf setup agent-apm' or an apm.yml registries: block.

Common patterns:
  $ jf agent apm update --dry-run
  $ jf agent apm update --yes --build-name=my-build --build-number=1

Gotchas:
- --yes is required to apply an update. Without it, update shows a plan and exits with an error instead of applying anything.
- A dependency pinned to a bare tag (#1.0.0) never has anything to update; only a semver range (#^1.0.0, #~1.0.0) can move to a newer matching version.
- --dry-run previews the plan without applying changes and skips build-info.
- Build-info is collected only when both --build-name and --build-number are provided; publish it afterwards with 'jf rt build-publish'.

Related: jf agent apm install, jf agent apm publish, jf setup agent-apm, jf rt build-publish`
}
