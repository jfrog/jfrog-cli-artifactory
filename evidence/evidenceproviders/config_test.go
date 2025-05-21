package evidenceproviders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestLoadConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	validConfig := `
sonar:
  url: "http://sonar.example.com"
  reportTaskFile: "/path/to/report"
  maxRetries: 5
  RetryInterval: 10
  proxy: "http://proxy.example.com"
jira:
  url: "http://jira.example.com"
`
	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	assert.NoError(t, err)

	config, err := LoadConfig(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, 2, len(config))
	assert.Contains(t, config, "sonar")
	assert.Contains(t, config, "jira")

	// Test case 2: File doesn't exist
	nonExistentPath := filepath.Join(tempDir, "non-existent.yaml")
	config, err = LoadConfig(nonExistentPath)
	assert.Error(t, err)
	assert.Nil(t, config)

	// Test case 3: Invalid YAML
	invalidYaml := "invalid: yaml: content: -"
	err = os.WriteFile(configPath, []byte(invalidYaml), 0644)
	assert.NoError(t, err)

	config, err = LoadConfig(configPath)
	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestGetConfig(t *testing.T) {
	// Setup: Create temp directory structure and config file
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	tempDir := t.TempDir()
	os.Chdir(tempDir)

	evidenceDir := filepath.Join(tempDir, ".jfrog", "evidence")
	err := os.MkdirAll(evidenceDir, 0755)
	assert.NoError(t, err)

	configPath := filepath.Join(evidenceDir, "evidence.yaml")
	validConfig := `
sonar:
  url: "http://sonar.example.com"
  reportTaskFile: "/path/to/report"
`
	err = os.WriteFile(configPath, []byte(validConfig), 0644)
	assert.NoError(t, err)

	// Test GetConfig
	config, err := GetConfig()
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Contains(t, config, "sonar")
}

func TestGetEvidenceDir(t *testing.T) {
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	tempDir, err := os.Getwd()
	os.Chdir(tempDir)

	localDir, err := GetEvidenceDir(false)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, ".jfrog", "evidence"), localDir)

	globalDir, err := GetEvidenceDir(true)
	assert.NoError(t, err)
	assert.NotEmpty(t, globalDir)
	assert.Contains(t, globalDir, "evidence")
}

func TestUnmarshalSonarConfig(t *testing.T) {
	yamlStr := `
url: "http://sonar.example.com"
reportTaskFile: "/path/to/report"
maxRetries: 5
RetryInterval: 10
proxy: "http://proxy.example.com"
`
	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlStr), &node)
	assert.NoError(t, err)

	// Create evidence config and unmarshal sonar config
	var evidenceConfig EvidenceConfig
	err = node.Decode(&evidenceConfig.Sonar)
	assert.NoError(t, err)

	// Verify fields
	assert.Equal(t, "http://sonar.example.com", evidenceConfig.Sonar.URL)
	assert.Equal(t, "/path/to/report", evidenceConfig.Sonar.ReportTaskFile)
	assert.NotNil(t, evidenceConfig.Sonar.MaxRetries)
	assert.Equal(t, 5, *evidenceConfig.Sonar.MaxRetries)
	assert.NotNil(t, evidenceConfig.Sonar.RetryInterval)
	assert.Equal(t, 10, *evidenceConfig.Sonar.RetryInterval)
	assert.Equal(t, "http://proxy.example.com", evidenceConfig.Sonar.Proxy)
}

func TestEvidenceConfig_Structures(t *testing.T) {
	maxRetries := 5
	retryInterval := 10

	config := EvidenceConfig{
		Sonar: &SonarConfig{
			URL:            "http://sonar.example.com",
			ReportTaskFile: "/path/to/report",
			MaxRetries:     &maxRetries,
			RetryInterval:  &retryInterval,
			Proxy:          "http://proxy.example.com",
		},
	}

	assert.Equal(t, "http://sonar.example.com", config.Sonar.URL)
	assert.Equal(t, "/path/to/report", config.Sonar.ReportTaskFile)
	assert.Equal(t, 5, *config.Sonar.MaxRetries)
	assert.Equal(t, 10, *config.Sonar.RetryInterval)
	assert.Equal(t, "http://proxy.example.com", config.Sonar.Proxy)
}
