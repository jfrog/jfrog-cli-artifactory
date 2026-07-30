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
  $ jf agent apm update
  $ jf agent apm update --build-name=my-build --build-number=1

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
