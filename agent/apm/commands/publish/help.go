package publish

func GetDescription() string {
	return "Publish an APM package to JFrog Artifactory."
}

func GetAIDescription() string {
	return `Publish an agent package to a JFrog Artifactory agentpackages repository with authenticated access.

When to use:
- Publishing custom agent packages for installation across multiple projects.
- Packaging skills, tools, or other agent extensions for organizational use.
- Creating reproducible, versioned deployments of agent components.

Prerequisites:
- apm CLI (>= 0.1.0) installed and in PATH.
- An apm.yml file in the package directory (or parent directories).
- Write permission on the Artifactory agentpackages repository.
- Registry configured via jf setup agent-apm or apm.yml's registries: block.

Common patterns:
  # Auto-pack apm.yml/.apm/ and publish
  $ jf agent apm publish --package my-org/my-package

  # Publish and record build-info
  $ jf agent apm publish --package my-org/my-package --build-name=my-build --build-number=1

  # Group multiple packages under one build-info module
  $ jf agent apm publish --package my-org/my-package --build-name=my-build --build-number=1 --module=my-module

  # Preview what would be uploaded without publishing anything
  $ jf agent apm publish --package my-org/my-package --dry-run

Note:
- --package is required and must be passed explicitly (owner/name); it is not inferred from a
  bare positional argument.

Package format:
- Directory with apm.yml declaring name, version, and description.
- Optional skills/ subdirectory containing Cursor Agent Skills.
- Version in apm.yml becomes the published package version.

Build info:
- Enabled with --build-name and --build-number flags.
- Captures package metadata and publishing source.
- Published to Artifactory for traceability and compliance.
- Optional --module to group multiple packages in the same build.

Environment:
- Credentials injected via APM_REGISTRY_TOKEN_<NAME>, APM_REGISTRY_USER_<NAME>, APM_REGISTRY_PASS_<NAME>.
- Registry configuration sourced from ~/.apm/config.json (set by jf setup agent-apm).

Related: jf agent apm install, jf agent apm update, jf setup agent-apm`
}
