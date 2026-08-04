package update

func GetDescription() string {
	return "Refresh APM dependencies to their latest matching refs, with build-info collection."
}

func GetAIDescription() string {
	return `Update packages in apm.yml to their latest matching versions and refresh the lockfile with authenticated access to JFrog Artifactory.

When to use:
- Keeping agent package dependencies up-to-date within declared version constraints.
- Re-resolving dependencies when new versions are published to the registry.
- Collecting updated build-info about package dependencies in CI/CD pipelines.

Prerequisites:
- apm CLI (>= 0.1.0) installed and in PATH.
- An apm.yml file in the working directory with dependencies declared.
- A lockfile apm.lock.yaml already created (e.g., via jf agent apm install).
- Read permission on the source Artifactory agentpackages repository.
- Registry configured via jf setup agent-apm or apm.yml's registries: block.

Common patterns:
  # Preview the update plan without applying anything
  $ jf agent apm update --dry-run

  # Apply the update plan and record build-info
  $ jf agent apm update --yes --build-name=my-build --build-number=1

Note:
- --yes is required to actually apply an update. update always shows a plan and asks for
  confirmation first; without --yes it exits with an error instead of applying anything,
  even when there's a real plan to apply.
- A dependency pinned to a bare tag (#1.0.0) never has anything to update - only a semver
  range (#^1.0.0, #~1.0.0) gives update room to move to a newer matching version.

Version constraints:
- Respects version constraints in apm.yml's dependencies section.
- Fetches latest versions matching declared constraints.
- Updates apm.lock.yaml with resolved versions and checksums.

Build info:
- Enabled with --build-name and --build-number flags.
- Captures updated packages and their transitive dependencies.
- Published to Artifactory for traceability and compliance.

Environment:
- Credentials injected via APM_REGISTRY_TOKEN_<NAME>, APM_REGISTRY_USER_<NAME>, APM_REGISTRY_PASS_<NAME>.
- Registry configuration sourced from ~/.apm/config.json (set by jf setup agent-apm).

Related: jf agent apm install, jf agent apm publish, jf setup agent-apm`
}
