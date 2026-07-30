package install

func GetDescription() string {
	return "Install APM packages with JFrog Artifactory authentication."
}

func GetAIDescription() string {
	return `Install packages declared in apm.yml with authenticated access to JFrog Artifactory registries.

When to use:
- Installing packages into an agent project that has apm.yml configured.
- Accessing private or curated packages from Artifactory via registry credentials.
- Collecting build-info about package dependencies in CI/CD pipelines.

Prerequisites:
- apm CLI (>= 0.1.0) installed and in PATH.
- A registry declared in apm.yml's registries: block or configured via jf setup agent-apm.
- Read permission on the source Artifactory agentpackages repository.

Common patterns:
  $ jf agent apm install
  $ jf agent apm install --build-name=my-build --build-number=1

Build info:
- Enabled with --build-name and --build-number flags.
- Captures installed packages and their transitive dependencies.
- Published to Artifactory for traceability and compliance.

Environment:
- Credentials injected via APM_REGISTRY_TOKEN_<NAME>, APM_REGISTRY_USER_<NAME>, APM_REGISTRY_PASS_<NAME>.
- Registry configuration sourced from ~/.apm/config.json (set by jf setup agent-apm).
- Lockfile apm.lock.yaml created in working directory.

Related: jf agent apm publish, jf agent apm update, jf setup agent-apm`
}
