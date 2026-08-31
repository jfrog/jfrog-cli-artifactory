package install

func GetDescription() string {
	return "Install APM packages with JFrog Artifactory authentication."
}

func GetAIDescription() string {
	return `Install packages declared in apm.yml with authenticated access to Artifactory agentpackages repositories, and optionally record a build-info of installed dependencies.

When to use:
- Installing packages into an agent project that has apm.yml configured.
- Pulling private or curated packages from Artifactory.
- Capturing build-info for an install by passing --build-name and --build-number.

Prerequisites:
- apm CLI installed and on PATH.
- A registry declared in apm.yml's registries: block, or configured via 'jf setup apm'.
- Read permission on the source Artifactory agentpackages repository.

Common patterns:
  $ jf agent apm install
  $ jf agent apm install my-org/my-package#1.0.0 --target claude
  $ jf agent apm install "my-org/my-package#^1.0.0" --target claude --build-name=my-build --build-number=1
  $ jf agent apm install --dev my-org/my-dev-tool#1.0.0
  $ jf agent apm install --dry-run

Gotchas:
- A bare tag (#1.0.0) is an exact pin: 'jf agent apm update' never moves it. Use a semver range (#^1.0.0, #~1.0.0) if later updates should pick up newer matching versions.
- --dry-run previews the install without changing anything and skips build-info (nothing real to record).
- Build-info is collected only when both --build-name and --build-number are provided; publish it afterwards with 'jf rt build-publish'.

Related: jf agent apm publish, jf setup apm, jf rt build-publish`
}
