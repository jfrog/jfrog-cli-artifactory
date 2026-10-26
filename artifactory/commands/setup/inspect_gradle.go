package setup

import (
	"regexp"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// gradleInitScriptDef matches the `def name = '...'` lines at the top of the init script
// template (gradle/resources/jfrog.init.gradle).
var gradleInitScriptDef = regexp.MustCompile(`(?m)^\s*def\s+(\w+)\s*=\s*'([^']*)'`)

func parseGradleInitScript(content []byte) map[string]string {
	values := map[string]string{}
	for _, match := range gradleInitScriptDef.FindAllStringSubmatch(string(content), -1) {
		values[match[1]] = match[2]
	}
	return values
}

func inspectGradle(serverDetails *config.ServerDetails) (inspection, error) {
	result := newInspection(binaryFound("gradle"))
	path := gradle.GetInitScriptPath()
	result.status.Location = path
	content, exists, err := readOptionalFile(path)
	if err != nil || !exists {
		return result, err
	}
	values := parseGradleInitScript(content)
	artifactoryURL := strings.TrimSpace(values["artifactoryUrl"])
	repoKey := strings.TrimSpace(values["gradleRepoName"])
	result.status.State, result.status.Host, _ = classify(project.Gradle, artifactoryURL, serverDetails, nil)
	if result.status.State != StateConfigured {
		return result, nil
	}
	result.status.RepoKey = repoKey
	result.status.Credentials = CredentialsAbsent
	if token := values["artifactoryAccessToken"]; token != "" {
		result.status.Credentials = CredentialsPresent
		result.credentials = storedCredentials{user: values["artifactoryUsername"], password: token}
	}
	return result, nil
}
