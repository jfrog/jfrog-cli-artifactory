package sonarqube

import (
	"encoding/json"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	evidence2 "github.com/jfrog/jfrog-client-go/evidence"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"os"
)

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

func CreateSonarQubeEvidence(serverDetails *config.ServerDetails) error {
	log.Debug("Collecting sonar evidence...")
	evd := &evidence2.EvidenceServicesManager{}
	evidenceData, err := evd.CreateSonarQubeEvidence()
	if err != nil {
		return err
	}
	taskReport := TaskReport{}
	err = json.Unmarshal(evidenceData, &taskReport)
	if err != nil {
		return err
	}
	log.Debug("Received sonar analysis ID: " + taskReport.Task.AnalysisID)
	qubeEvidence, err := evd.GetSonarQubeProjectStatus(taskReport.Task.AnalysisID)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp("", "evidence_*.json")
	if err != nil {
		return err
	}
	defer tmpFile.Close()
	_, err = tmpFile.Write(qubeEvidence)
	if err != nil {
		log.Error("failed to write evidence to temporary file: " + err.Error())
		return err
	}
	log.Debug("Evidence written to temporary file: " + tmpFile.Name())

	predicateFilePath := tmpFile.Name()
	//predicateFilePath := "/Users/bhanur/codebase/resources/sonar_report.json"
	serverDetails.EvidenceUrl = "http://localhost:8082/evidence"
	evdCreateCmd := evidence.NewCreateEvidenceCustom(serverDetails,
		predicateFilePath,
		"https://jfrog.com/evidence/sonarqube/v1",
		"",
		"/Users/bhanur/codebase/scripts/private.pem",
		"evidence-local",
		"dev-maven-local/com/example/demo-sonar/1.0/demo-sonar-1.0.jar",
		"",
	)
	log.Debug("Creating Sonar Evidence...")
	err = evdCreateCmd.Run()
	if err != nil {
		log.Error("failed to create evidence: " + err.Error())
	}
	return nil
}
