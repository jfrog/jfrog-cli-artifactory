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
  # Install everything already declared in apm.yml
  $ jf agent apm install

  # Add and install a package at an exact version
  $ jf agent apm install my-org/my-package#1.0.0 --target claude

  # Install a floating version range and record build-info
  $ jf agent apm install "my-org/my-package#^1.0.0" --target claude --build-name=my-build --build-number=1

  # Preview what would be installed without installing anything
  $ jf agent apm install --dry-run

  # Add a dev-only dependency
  $ jf agent apm install --dev my-org/my-dev-tool#1.0.0

Note:
- A bare tag (#1.0.0) is an exact pin: apm update never moves it. Use a semver range
  (#^1.0.0, #~1.0.0) if you want later updates to pick up newer matching versions.
- --dry-run shows what would be installed without installing, and skips build-info
  entirely (there's nothing real to record). --global installs to ~/.apm instead of the
  current project and also skips build-info, since it isn't scoped to any one project.

Build info:
- Enabled with --build-name and --build-number flags.
- Captures installed packages and their transitive dependencies.
- Published to Artifactory for traceability and compliance.

Environment:
- Credentials injected via APM_REGISTRY_TOKEN_<NAME>, APM_REGISTRY_USER_<NAME>, APM_REGISTRY_PASS_<NAME>.
- Registry configuration sourced from ~/.apm/config.json (set by jf setup agent-apm).
- Lockfile apm.lock.yaml created in the working directory (or under --root, if passed).

Related: jf agent apm publish, jf agent apm update, jf setup agent-apm`
}
