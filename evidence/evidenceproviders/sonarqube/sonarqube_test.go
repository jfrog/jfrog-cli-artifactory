package sonarqube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/evidenceproviders"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var (
	getConfigFunc                   = evidenceproviders.GetConfig
	fetchSonarEvidenceWithRetryFunc = FetchSonarEvidenceWithRetry
)

func TestNewSonarConfig(t *testing.T) {
	config := NewSonarConfig("http://sonar.example.com", "/path/to/report", "5", "10", "http://proxy.example.com")
	assert.Equal(t, "http://sonar.example.com", config.URL)
	assert.Equal(t, "/path/to/report", config.ReportTaskFile)
	assert.Equal(t, 5, *config.MaxRetries)
	assert.Equal(t, 10, *config.RetryInterval)
	assert.Equal(t, "http://proxy.example.com", config.Proxy)

	config = NewSonarConfig("http://sonar.example.com", "/path/to/report", "invalid", "10", "http://proxy.example.com")
	assert.Equal(t, 0, *config.MaxRetries)

	config = NewSonarConfig("http://sonar.example.com", "/path/to/report", "5", "invalid", "http://proxy.example.com")
	assert.Equal(t, 0, *config.RetryInterval)
}

func TestNewDefaultSonarConfig(t *testing.T) {
	config := NewDefaultSonarConfig()
	assert.Equal(t, DefaultSonarHost, config.URL)
	assert.Equal(t, DefaultReportTaskFile, config.ReportTaskFile)
	assert.Equal(t, DefaultRetries, *config.MaxRetries)
	assert.Equal(t, DefaultIntervalInSeconds, *config.RetryInterval)
	assert.Equal(t, "", config.Proxy)
}

func TestCreateSonarConfiguration(t *testing.T) {
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

	config, err := CreateSonarConfiguration(&node)
	assert.NoError(t, err)
	assert.Equal(t, "http://sonar.example.com", config.URL)
	assert.Equal(t, "/path/to/report", config.ReportTaskFile)
	assert.Equal(t, 5, *config.MaxRetries)
	assert.Equal(t, 10, *config.RetryInterval)
	assert.Equal(t, "http://proxy.example.com", config.Proxy)

	var invalidNode yaml.Node
	invalidNode.Kind = yaml.ScalarNode
	_, err = CreateSonarConfiguration(&invalidNode)
	assert.Error(t, err)
}

func TestGetCeTaskUrlFromFile(t *testing.T) {
	tempDir := t.TempDir()
	validFilePath := filepath.Join(tempDir, "valid-report-task.txt")
	validContent := `
projectKey=my-project
serverUrl=https://sonarcloud.io
serverVersion=8.0
dashboardUrl=https://sonarcloud.io/dashboard?id=my-project
ceTaskId=task-id-123
ceTaskUrl=https://sonarcloud.io/api/ce/task?id=task-id-123
`
	err := os.WriteFile(validFilePath, []byte(validContent), 0644)
	assert.NoError(t, err)

	_, taskID, err := getCeTaskIDAndURLFromReportTaskFile(validFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "task-id-123", taskID)

	invalidFilePath := filepath.Join(tempDir, "invalid-report-task.txt")
	invalidContent := `
projectKey=my-project
serverUrl=https://sonarcloud.io
serverVersion=8.0
dashboardUrl=https://sonarcloud.io/dashboard?id=my-project
`
	err = os.WriteFile(invalidFilePath, []byte(invalidContent), 0644)
	assert.NoError(t, err)

	_, _, err = getCeTaskIDAndURLFromReportTaskFile(invalidFilePath)
	assert.Error(t, err)

	malformedFilePath := filepath.Join(tempDir, "malformed-report-task.txt")
	malformedContent := `
projectKey=my-project
serverUrl=https://sonarcloud.io
ceTaskUrl=malformed-url-without-id
`
	err = os.WriteFile(malformedFilePath, []byte(malformedContent), 0644)
	assert.NoError(t, err)

	_, _, err = getCeTaskIDAndURLFromReportTaskFile(malformedFilePath)
	assert.Error(t, err)

	_, _, err = getCeTaskIDAndURLFromReportTaskFile(filepath.Join(tempDir, "non-existent.txt"))
	assert.Error(t, err)
}

func TestCreateSonarEvidence(t *testing.T) {
	originalGetConfig := getConfigFunc
	defer func() { getConfigFunc = originalGetConfig }()

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

	getConfigFunc = func() (map[string]*yaml.Node, error) {
		return map[string]*yaml.Node{"sonar": &node}, nil
	}

	sonarEvidence, err := CreateSonarEvidence()
	assert.NoError(t, err)
	assert.NotNil(t, sonarEvidence)
	assert.Equal(t, "http://sonar.example.com", sonarEvidence.SonarConfig.URL)
	assert.Equal(t, "/path/to/report", sonarEvidence.SonarConfig.ReportTaskFile)
	assert.Equal(t, 5, *sonarEvidence.SonarConfig.MaxRetries)
	assert.Equal(t, 10, *sonarEvidence.SonarConfig.RetryInterval)
	assert.Equal(t, "http://proxy.example.com", sonarEvidence.SonarConfig.Proxy)

	getConfigFunc = func() (map[string]*yaml.Node, error) {
		return nil, assert.AnError
	}
	_, err = CreateSonarEvidence()
	assert.Error(t, err)

	getConfigFunc = func() (map[string]*yaml.Node, error) {
		return map[string]*yaml.Node{}, nil
	}
	_, err = CreateSonarEvidence()
	assert.Error(t, err)
}

func TestFetchSonarEvidenceWithRetry(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ce/task" {
			taskID := r.URL.Query().Get("id")
			if taskID == "task-success" {
				task := TaskReport{
					Task: Task{
						ID:         "task-success",
						Status:     "SUCCESS",
						AnalysisID: "analysis-123",
					},
				}
				json.NewEncoder(w).Encode(task)
			} else if taskID == "task-pending" {
				task := TaskReport{
					Task: Task{
						ID:     "task-pending",
						Status: "PENDING",
					},
				}
				json.NewEncoder(w).Encode(task)
			} else if taskID == "task-progress" {
				task := TaskReport{
					Task: Task{
						ID:     "task-progress",
						Status: "IN-PROGRESS",
					},
				}
				json.NewEncoder(w).Encode(task)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		} else if r.URL.Path == "/api/qualitygates/project_status" {
			analysisID := r.URL.Query().Get("analysisId")
			if analysisID == "analysis-123" {
				w.Write([]byte(`{"projectStatus":{"status":"OK"}}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()

	reportTaskPath := filepath.Join(tempDir, "report-task.txt")
	reportContent := `ceTaskUrl=` + mockServer.URL + `/api/ce/task?id=task-success`
	err := os.WriteFile(reportTaskPath, []byte(reportContent), 0644)
	assert.NoError(t, err)

	data, err := FetchSonarEvidenceWithRetry(mockServer.URL, reportTaskPath, "", 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), `"status":"OK"`)

	pendingTaskPath := filepath.Join(tempDir, "pending-task.txt")
	pendingContent := `ceTaskUrl=` + mockServer.URL + `/api/ce/task?id=task-pending`
	err = os.WriteFile(pendingTaskPath, []byte(pendingContent), 0644)
	assert.NoError(t, err)

	_, err = FetchSonarEvidenceWithRetry(mockServer.URL, pendingTaskPath, "", 2, 1)
	assert.Error(t, err)

	_, err = FetchSonarEvidenceWithRetry(mockServer.URL, filepath.Join(tempDir, "non-existent.txt"), "", 1, 1)
	assert.Error(t, err)
}

func TestSonarEvidence_GetEvidence(t *testing.T) {
	maxRetries := 3
	retryInterval := 5
	sonarEvidence := &SonarEvidence{
		SonarConfig: &evidenceproviders.SonarConfig{
			URL:            "http://sonar.example.com",
			ReportTaskFile: "report-task.txt",
			MaxRetries:     &maxRetries,
			RetryInterval:  &retryInterval,
			Proxy:          "http://proxy.example.com",
		},
	}

	originalFunc := fetchSonarEvidenceWithRetryFunc
	defer func() { fetchSonarEvidenceWithRetryFunc = originalFunc }()

	fetchSonarEvidenceWithRetryFunc = func(sonarQubeURL, reportTaskFile, proxy string, maxRetries, retryInterval int) ([]byte, error) {
		return []byte(`{"projectStatus":{"status":"OK"}}`), nil
	}

	evidence, err := sonarEvidence.GetEvidence()
	assert.NoError(t, err)
	assert.Equal(t, `{"projectStatus":{"status":"OK"}}`, string(evidence))

	fetchSonarEvidenceWithRetryFunc = func(sonarQubeURL, reportTaskFile, proxy string, maxRetries, retryInterval int) ([]byte, error) {
		return nil, assert.AnError
	}

	_, err = sonarEvidence.GetEvidence()
	assert.Error(t, err)
}
