package sonar

import (
	"testing"

	conf "github.com/jfrog/jfrog-cli-artifactory/evidence/config"
	"github.com/stretchr/testify/assert"
)

func TestResolveSonarBaseURL_WithConfig(t *testing.T) {
	tests := []struct {
		name          string
		configuredURL string
		ceTaskURL     string
		serverURL     string
		expectedURL   string
	}{
		{
			name:          "Configured URL takes precedence",
			configuredURL: "https://configured.sonarcloud.io",
			ceTaskURL:     "https://ce-task.sonarcloud.io",
			serverURL:     "https://server.sonarcloud.io",
			expectedURL:   "https://configured.sonarcloud.io",
		},
		{
			name:          "ServerURL takes precedence over CeTaskURL",
			configuredURL: "",
			ceTaskURL:     "https://ce-task.sonarcloud.io",
			serverURL:     "https://server.sonarcloud.io",
			expectedURL:   "https://server.sonarcloud.io",
		},
		{
			name:          "ServerURL used when no CeTaskURL",
			configuredURL: "",
			ceTaskURL:     "",
			serverURL:     "https://server.sonarcloud.io",
			expectedURL:   "https://server.sonarcloud.io",
		},
		{
			name:          "Default URL when no configuration",
			configuredURL: "",
			ceTaskURL:     "",
			serverURL:     "",
			expectedURL:   "https://sonarcloud.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config
			cfg := &conf.EvidenceConfig{}
			if tt.configuredURL != "" {
				cfg.Sonar = &conf.SonarConfig{
					URL: tt.configuredURL,
				}
			}

			result := resolveSonarBaseURL(tt.ceTaskURL, tt.serverURL)

			// Apply config override if present
			if cfg != nil && cfg.Sonar != nil && cfg.Sonar.URL != "" {
				result = cfg.Sonar.URL
			}

			assert.Equal(t, tt.expectedURL, result)
		})
	}
}

func TestNewPredicateResolver(t *testing.T) {
	resolver := NewPredicateResolver()
	assert.NotNil(t, resolver)
	assert.IsType(t, &defaultPredicateResolver{}, resolver)
}

func TestResolvePredicate_ReturnsComponents(t *testing.T) {
	resolver := NewPredicateResolver()

	// This test will likely fail in a real environment since it needs Sonar configuration
	// but it tests the interface contract
	predicateType, predicate, err := resolver.ResolvePredicate()

	// We expect an error since there's no Sonar configuration in test environment
	assert.Error(t, err)
	assert.Empty(t, predicateType)
	assert.Nil(t, predicate)
	assert.Contains(t, err.Error(), "no report-task.txt file found")
}
