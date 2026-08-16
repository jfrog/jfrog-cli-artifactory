package publish

func GetDescription() string {
	return "Publish an APM package to JFrog Artifactory."
}

func GetAIDescription() string {
	return `Publish an agent package to an Artifactory agentpackages repository with authenticated access, and optionally record a build-info of the published package.

When to use:
- Publishing custom agent packages (skills, tools, extensions) for reuse across projects.
- Creating versioned, reproducible deployments of agent components.
- Capturing build-info for a publish by passing --build-name and --build-number.

Prerequisites:
- apm CLI installed and on PATH.
- An apm.yml in the package directory (or a parent) declaring name, version, and description.
- Write permission on the Artifactory agentpackages repository.
- Registry configured via 'jf setup apm' or an apm.yml registries: block.

Common patterns:
  $ jf agent apm publish --package my-org/my-package
  $ jf agent apm publish --package my-org/my-package --build-name=my-build --build-number=1
  $ jf agent apm publish --package my-org/my-package --build-name=my-build --build-number=1 --module=my-module
  $ jf agent apm publish --package my-org/my-package --dry-run

Gotchas:
- --package is required (owner/name); it is not inferred from a bare positional argument.
- --dry-run previews the upload without publishing and skips build-info.
- Build-info is collected only when both --build-name and --build-number are provided; optional --module groups packages under one module. Publish afterwards with 'jf rt build-publish'.

Related: jf agent apm install, jf agent apm update, jf setup apm, jf rt build-publish`
}
