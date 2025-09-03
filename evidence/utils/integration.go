package utils

import "strings"

const SonarIntegration = "sonar"

func IsSonarIntegration(integration string) bool {
	return strings.ToLower(integration) == SonarIntegration
}
