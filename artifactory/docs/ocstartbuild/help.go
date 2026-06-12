package ocstartbuild

var Usage = []string{"rt oc start-build <build config name | --from-build=<build name>> --repo=<target repository> [command options]"}

func GetDescription() string {
	return "Run OpenShift CLI (oc) start-build command."
}

func GetAIDescription() string {
	return `Wrap "oc start-build" (OpenShift CLI) so the resulting image push is captured in JFrog CLI build-info. Only the start-build subcommand is supported.

When to use:
- Triggering an OpenShift BuildConfig from a JFrog-managed pipeline.
- Recording the image pushed by OpenShift in a build-info for promote/scan.

Prerequisites:
- oc installed and authenticated to the OpenShift cluster.
- The BuildConfig must already exist on the cluster.
- --repo is mandatory; provide the Artifactory Docker repo target.
- Configured server (--server-id) for build-info publishing.

Common patterns:
  $ jf rt oc start-build my-build-config --repo=docker-local --server-id=my-prod --build-name=my-build --build-number=42

Gotchas:
- Anything other than "start-build" after "oc" is rejected.
- --repo and --server-id are stripped before forwarding; everything else is passed verbatim to oc.
- Failing oc invocations propagate as JFrog CLI errors; capture stderr for the underlying message.

Related: jf rt build-publish, jf rt build-docker-create`
}
