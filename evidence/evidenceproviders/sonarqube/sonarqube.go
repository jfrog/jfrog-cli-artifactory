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
	"github.com/jfrog/jfrog-client-go/evidence/external/sonarqube"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultSonarHost         = "https://sonarcloud.io"
	DefaultReportTaskFile    = ".scannerwork/report-task.txt" // target/sonar/report-task.txt
	DefaultRetries           = 3
	DefaultIntervalInSeconds = 10
	SonarTaskStatusSuccess   = "SUCCESS"
)

var (
	getConfigFunc                   = evidenceproviders.GetConfig
	fetchSonarEvidenceWithRetryFunc = FetchSonarEvidenceWithRetry
)

type SonarEvidence struct {
	ServerDetails *config.ServerDetails
	SonarConfig   *evidenceproviders.SonarConfig
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

func NewSonarConfig(url, reportTaskFile, maxRetries, retryInterval, proxy string) *evidenceproviders.SonarConfig {
	log.Debug("Creating sonarqube config: URL: " + url + " reportTaskFile: " + reportTaskFile + " maxRetries: " + maxRetries + " retryInterval: " + retryInterval)
	var retriesAllowed, retryCoolingPeriodSecs *int
	retries, err := strconv.Atoi(maxRetries)
	if err != nil {
		log.Warn("Invalid maxRetries config, using default of 0")
		retries = 0
	}
	retriesAllowed = &retries
	retryIntervalSecs, err := strconv.Atoi(retryInterval)
	if err != nil {
		log.Warn("Invalid retryInterval config, using default of 0")
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
		if errors.Is(err, evidenceproviders.ErrEvidenceDirNotExist) {
			log.Debug("No external evidence provider config found, using default sonar config")
			return &SonarEvidence{SonarConfig: NewDefaultSonarConfig()}, nil
		}
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
	interval := func() *int { v := DefaultIntervalInSeconds; return &v }()
	reportTaskFilePath := DetectBuildToolAndReportFilePath()
	taskURL, _ := GetSonarHostURLTaskIDFromReportTaskFile(reportTaskFilePath)
	return &evidenceproviders.SonarConfig{
		URL:            taskURL,
		ReportTaskFile: reportTaskFilePath,
		MaxRetries:     retries,
		RetryInterval:  interval,
		Proxy:          "",
	}
}

func (se *SonarEvidence) GetEvidence() ([]byte, error) {
	err := validateSonarConfig(se)
	if err != nil {
		return nil, err
	}
	log.Debug("Retrieving evidence from sonarqube server ", se.SonarConfig.URL, se.SonarConfig.Proxy)
	sonarReport, err := fetchSonarEvidenceWithRetryFunc(
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
	return sonarReport, nil
}

func validateSonarConfig(se *SonarEvidence) error {
	if se.SonarConfig == nil {
		se.SonarConfig = new(evidenceproviders.SonarConfig)
	}
	if se.SonarConfig.ReportTaskFile == "" {
		se.SonarConfig.ReportTaskFile = DetectBuildToolAndReportFilePath()
	}
	if se.SonarConfig.MaxRetries == nil {
		se.SonarConfig.MaxRetries = func() *int { v := DefaultRetries; return &v }()
	}
	if se.SonarConfig.RetryInterval == nil {
		se.SonarConfig.RetryInterval = func() *int { v := DefaultIntervalInSeconds; return &v }()
	}
	if err := validateSonarAccessToken(); err != nil {
		return err
	}
	return nil
}

func validateSonarAccessToken() error {
	sonarQubeToken := os.Getenv(sonarqube.SonarAccessTokenKey)
	if sonarQubeToken == "" {
		return errorutils.CheckErrorf("Sonar access token not found in environment variable " + sonarqube.SonarAccessTokenKey)
	}
	return nil
}

func CreateSonarConfiguration(yamlNode *yaml.Node) (sonarConfig *evidenceproviders.SonarConfig, err error) {
	if yamlNode == nil {
		return nil, errorutils.CheckError(errors.New("sonar config is empty"))
	}
	if err := yamlNode.Decode(&sonarConfig); err != nil {
		return nil, err
	}
	log.Debug("Reading sonarqube config", sonarConfig)
	return sonarConfig, nil
}

// FetchSonarEvidenceWithRetry fetches the sonar evidence using the sonar configuration.
// Reads report-task.txt and fetches taskURL and taskID
// It retries the request if it fails or if the task is still in progress or pending depending on the sonar config.
// It returns the evidence data if the task is successful or an error if it fails.
func FetchSonarEvidenceWithRetry(sonarQubeURL, reportTaskFile, proxy string, maxRetries, retryInterval int) (data []byte, err error) {
	taskURL, taskID := GetSonarHostURLTaskIDFromReportTaskFile(reportTaskFile)
	if sonarQubeURL == "" {
		sonarQubeURL = taskURL
	}
	log.Debug(fmt.Sprintf("Fetching sonarqube task status using taskID %s sonarqube URL %s", taskID, sonarQubeURL))
	if taskID == "" {
		return nil, errorutils.CheckError(errors.New("unable to determine task ID from report task file: " + reportTaskFile))
	}
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
			} else if taskReport.Task.Status == SonarTaskStatusSuccess {
				return false, nil
			}
			return true, nil
		},
	}
	err = retryExecutor.Execute()
	if err != nil {
		return nil, err
	}
	if taskReport.Task.Status != SonarTaskStatusSuccess {
		return nil, errorutils.CheckError(errors.New("Sonar task with unexpected status: " + taskReport.Task.Status))
	}
	return evd.GetSonarAnalysisReport(taskReport.Task.AnalysisID, sonarQubeURL, proxy)
}

func parseSonarHostURLFromTaskURL(taskURL string) string {
	parsedURL, err := url.Parse(taskURL)
	if err != nil {
		log.Debug("Failed to parse sonar URL from report-task", taskURL, "setting to default host")
		return DefaultSonarHost
	}
	return parsedURL.Scheme + "://" + parsedURL.Host
}

func getCeTaskIDAndURLFromReportTaskFile(filePath string) (string, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", errorutils.CheckError(errors.New("failed to open file: " + err.Error()))
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
				return "", "", errorutils.CheckError(errors.New("invalid ceTaskUrl format in file"))
			}
			taskIDs[0] = strings.TrimPrefix(taskIDs[0], "ceTaskUrl=")
			return taskIDs[0], taskIDs[1], nil
		}
	}
	return "", "", errorutils.CheckError(errors.New("ceTaskUrl not found in file"))
}

func GetSonarHostURLTaskIDFromReportTaskFile(reportTaskFilePath string) (string, string) {
	sonarURL, taskID, err := getCeTaskIDAndURLFromReportTaskFile(reportTaskFilePath)
	if err != nil {
		log.Warn(err.Error(), "falling back to default url")
		return DefaultSonarHost, taskID
	}
	return parseSonarHostURLFromTaskURL(sonarURL), taskID
}
