package utils

import "fmt"

func BuildBuildInfoRepoKey(project string) string {
	if project == "" || project == "default" {
		return "artifactory-build-info"
	}
	return fmt.Sprintf("%s-build-info", project)
}

func BuildReleaseBundleRepoKey(project string) string {
	if project == "" || project == "default" {
		return "release-bundles-v2"
	}
	return fmt.Sprintf("%s-release-bundles-v2", project)
}
