package nuget

var Usage = []string{"rt ndt"}

func GetDescription() string {
	return "Show solution dependency tree."
}

func GetAIDescription() string {
	return `Print the .NET/NuGet solution dependency tree from the current directory. Local-only inspection helper; does not contact Artifactory.

When to use:
- Auditing transitive NuGet dependencies before publishing.
- Generating a quick text view of the package graph for a .NET project.

Prerequisites:
- Run from the directory containing a .sln or .csproj file with restored packages.
- dotnet/nuget restore should have been run already.

Common patterns:
  $ jf rt nuget-deps-tree

Gotchas:
- Reads from the local project; missing/incomplete restore yields an empty/partial tree.
- Output is plain text on stdout; not machine-parsable.

Related: jf rt nuget, jf rt build-publish`
}
