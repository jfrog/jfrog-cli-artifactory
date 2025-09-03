package utils

import (
	"fmt"
	"strings"
)

const SonarIntegration = "sonar"

func IsSonarIntegration(integration string) bool {
	return strings.ToLower(integration) == SonarIntegration
}

func ValidateIntegration(integration string) error {
	if integration != "" && !IsSonarIntegration(integration) {
		return fmt.Errorf("integration %s does not exist", integration)
	}
	return nil
}
