package utils

import "strings"

const SonarIntegration = "sonar"

func IsSonarIntegration(provider string) bool {
	return strings.ToLower(provider) == SonarIntegration
}
