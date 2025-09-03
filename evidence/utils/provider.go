package utils

import "strings"

const SonarProvider = "sonar"

func IsSonarProvider(provider string) bool {
	return strings.ToLower(provider) == SonarProvider
}
