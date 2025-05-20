package sonarqube

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/evidenceproviders"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/evidence"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultSonarHost      = "https://sonarcloud.io"
	DefaultReportTaskFile = "target/sonar/report-task.txt"
	DefaultRetries        = 3
	DefaultInterval       = 10
)

type SonarEvidence struct {
	ServerDetails *config.ServerDetails
	SonarConfig   *evidenceproviders.SonarConfig
}

func NewSonarConfig(url, reportTaskFile, maxRetries, retryInterval, proxy string) *evidenceproviders.SonarConfig {
	log.Debug("Creating sonarqube config URL: " + url + " reportTaskFile: " + reportTaskFile + " maxRetries: " + maxRetries + " retryInterval: " + retryInterval)
	var retriesAllowed, retryCoolingPeriodSecs *int
	retries, err := strconv.Atoi(maxRetries)
	if err != nil {
		log.Warn("Invalid maxRetries value, using default")
		retries = 0
	}
	retriesAllowed = &retries
	retryIntervalSecs, err := strconv.Atoi(retryInterval)
	if err != nil {
		log.Warn("Invalid retryInterval value, using default")
		retryIntervalSecs = 0
	}
	retryCoolingPeriodSecs = &retryIntervalSecs
	return &evidenceproviders.SonarConfig{
		URL:            url,
		ReportTaskFile: reportTaskFile,
		MaxRetries:     retriesAllowed,
		RetryInterval:  retryCoolingPeriodSecs,
		Proxy:          proxy,
	}
}

// CreateSonarEvidence creates the evidence using the sonar configuration.
// It reads the sonar configuration from the evidence.yaml file in the .jfrog/evidence directory.
// It filters the sonar configuration to only include the fields that are needed for the sonar evidence.
func CreateSonarEvidence() (*SonarEvidence, error) {
	externalEvidenceProviderConfig, err := evidenceproviders.GetConfig()
	if err != nil {
		return nil, err
	}
	sonarConfig, err := CreateSonarConfiguration(externalEvidenceProviderConfig["sonar"])
	if err != nil {
		return nil, err
	}
	return &SonarEvidence{SonarConfig: sonarConfig}, nil
}

func NewDefaultSonarConfig() *evidenceproviders.SonarConfig {
	retries := func() *int { v := DefaultRetries; return &v }()
	interval := func() *int { v := DefaultInterval; return &v }()
	return &evidenceproviders.SonarConfig{
		URL:            DefaultSonarHost,
		ReportTaskFile: DefaultReportTaskFile,
		MaxRetries:     retries,
		RetryInterval:  interval,
		Proxy:          "",
	}
}

func (se *SonarEvidence) GetEvidence() ([]byte, error) {
	log.Info("Retrieving evidence from sonarqube server")
	sonarEvidence, err := FetchSonarEvidenceWithRetry(
		se.SonarConfig.URL,
		se.SonarConfig.ReportTaskFile,
		se.SonarConfig.Proxy,
		*se.SonarConfig.MaxRetries,
		*se.SonarConfig.RetryInterval,
	)
	if err != nil {
		return nil, err
	}
	log.Info("Fetched sonar evidence successfully")
	return sonarEvidence, nil
}

func CreateSonarConfiguration(yamlNode *yaml.Node) (sonarConfig *evidenceproviders.SonarConfig, err error) {
	if err := yamlNode.Decode(&sonarConfig); err != nil {
		return nil, err
	}
	log.Debug("Reading sonarqube config", sonarConfig)
	return sonarConfig, nil
}

type TaskReport struct {
	Task Task `json:"task"`
}

type Task struct {
	ID                 string   `json:"id"`
	Type               string   `json:"type"`
	ComponentID        string   `json:"componentId"`
	ComponentKey       string   `json:"componentKey"`
	ComponentName      string   `json:"componentName"`
	ComponentQualifier string   `json:"componentQualifier"`
	AnalysisID         string   `json:"analysisId"`
	Status             string   `json:"status"`
	SubmittedAt        string   `json:"submittedAt"`
	SubmitterLogin     string   `json:"submitterLogin"`
	StartedAt          string   `json:"startedAt"`
	ExecutedAt         string   `json:"executedAt"`
	ExecutionTimeMs    int      `json:"executionTimeMs"`
	HasScannerContext  bool     `json:"hasScannerContext"`
	WarningCount       int      `json:"warningCount"`
	Warnings           []string `json:"warnings"`
	InfoMessages       []string `json:"infoMessages"`
}

func getCeTaskUrlFromFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		panic("Failed to open file: " + err.Error())
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error("Failed to close file: " + err.Error())
		}
	}(file)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ceTaskUrl=") {
			taskIDs := strings.Split(line, "?id=")
			if len(taskIDs) < 2 {
				log.Error("Invalid ceTaskUrl format in file")
				return "", errorutils.CheckError(errors.New("invalid ceTaskUrl format in file"))
			}
			return strings.Split(line, "?id=")[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		panic("Error reading file: " + err.Error())
	}

	log.Error("ceTaskUrl not found in file")
	return "", errorutils.CheckError(errors.New("ceTaskUrl not found in file"))
}

// FetchSonarEvidenceWithRetry fetches the sonar evidence using the sonar configuration.
// It retries the request if it fails or if the task is still in progress or pending depending on the sonar config.
// It returns the evidence data if the task is successful or an error if it fails.
func FetchSonarEvidenceWithRetry(sonarQubeURL, reportTaskFile, proxy string, maxRetries, retryInterval int) (data []byte, err error) {
	taskID, err := getCeTaskUrlFromFile(reportTaskFile)
	if err != nil {
		return data, err
	}
	log.Debug(fmt.Sprintf("Fetching sonarqube task status using taskID %s", taskID))
	evd := &evidence.EvidenceServicesManager{}
	var taskReport *TaskReport
	retryExecutor := utils.RetryExecutor{
		Context:                  context.Background(),
		MaxRetries:               maxRetries,
		RetriesIntervalMilliSecs: retryInterval * 1000,
		ExecutionHandler: func() (shouldRetry bool, err error) {
			taskReport = new(TaskReport)
			evidenceData, err := evd.FetchSonarTaskStatus(taskID, sonarQubeURL, proxy)
			if err != nil || evidenceData == nil {
				return true, err
			}
			err = json.Unmarshal(evidenceData, &taskReport)
			if err != nil {
				return true, err
			}
			if taskReport.Task.Status == "PENDING" || taskReport.Task.Status == "IN-PROGRESS" {
				return true, nil
			} else if taskReport.Task.Status == "SUCCESS" {
				return false, nil
			}
			return true, nil
		},
	}
	err = retryExecutor.Execute()
	if err != nil {
		return nil, err
	}
	return evd.GetSonarAnalysisReport(taskReport.Task.AnalysisID, sonarQubeURL, proxy)
}
