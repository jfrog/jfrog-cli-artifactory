package export

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbe <release bundle name> <release bundle version> [target pattern]"}

func GetDescription() string {
	return "Triggers the Export process and downloads the Release Bundle archive"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "release bundle name", Description: "Name of the Release Bundle to export."},
		{Name: "release bundle version", Description: "Version of the Release Bundle to export."},
		{Name: "target pattern", Description: "The third argument is optional and specifies the local file system target path.\n\t\tIf the target path ends with a slash, the path is assumed to be a directory.\n\t\tFor example, if you specify the target as \"repo-name/a/b/\", then \"b\" is assumed to be a directory into which files should be downloaded.\n\t\tIf there is no terminal slash, the target path is assumed to be a file to which the downloaded file should be renamed.\n\t\tFor example, if you specify the target as \"a/b\", the downloaded file is renamed to \"b\"."},
	}
}
