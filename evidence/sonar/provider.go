package sonar

import (
	"encoding/json"
	"time"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const jfrogPredicateType = "https://jfrog.com/evidence/sonarqube/v1"

// Set default values if not provided
const defaultMaxRetries = 30
const defaultRetryInterval = 5000 // in milliseconds

// Predicate model for SonarQube analysis results
type sonarPredicate struct {
	Gates []struct {
		Type              string `json:"type"`
		Status            string `json:"status"`
		IgnoredConditions bool   `json:"ignoredConditions"`
		Conditions        []struct {
			Status         string `json:"status"`
			MetricKey      string `json:"metricKey"`
			Comparator     string `json:"comparator"`
			ErrorThreshold string `json:"errorThreshold"`
			ActualValue    string `json:"actualValue"`
		} `json:"conditions"`
	} `json:"gates"`
}

// Provider handles SonarQube analysis business logic
type Provider struct {
	client           Client
	cachedAnalysisId string
}

// NewSonarProviderWithCredentials creates a new SonarProvider with SonarQube credentials
func NewSonarProviderWithCredentials(sonarURL, token string) (*Provider, error) {
	if sonarURL == "" {
		return nil, errorutils.CheckErrorf("SonarQube URL is required")
	}
	if token == "" {
		return nil, errorutils.CheckErrorf("SonarQube token is required")
	}

	return &Provider{
		client: NewClient(sonarURL, token),
	}, nil
}

// BuildPredicate the fallback flow: build predicate from quality gates.
func (p *Provider) BuildPredicate(ceTaskID string, maxRetries *int, retryInterval *int) ([]byte, string, error) {
	if ceTaskID == "" {
		return nil, "", errorutils.CheckErrorf("ceTaskID is required for SonarQube evidence creation")
	}

	if p.cachedAnalysisId == "" {
		log.Info("Polling for task completion:", ceTaskID)
		completedAnalysisID, err := p.pollTaskUntilSuccess(ceTaskID, maxRetries, retryInterval)
		if err != nil {
			return nil, "", errorutils.CheckErrorf("failed to poll task completion: %v", err)
		}
		p.cachedAnalysisId = completedAnalysisID
	}

	predicate, err := p.buildPredicateFromQualityGates()
	if err != nil {
		return nil, "", err
	}
	return predicate, jfrogPredicateType, nil
}

// BuildStatement tries to retrieve an in-toto statement from the integration endpoint.
// If successful, returns the statement bytes.
func (p *Provider) BuildStatement(ceTaskID string, maxRetries *int, retryInterval *int) ([]byte, error) {
	if ceTaskID == "" {
		return nil, errorutils.CheckErrorf("ceTaskID is required for SonarQube evidence creation")
	}

	if p.cachedAnalysisId == "" {
		log.Info("Polling for task completion:", ceTaskID)
		completedAnalysisID, err := p.pollTaskUntilSuccess(ceTaskID, maxRetries, retryInterval)
		if err != nil {
			return nil, errorutils.CheckErrorf("failed to poll task completion: %v", err)
		}
		p.cachedAnalysisId = completedAnalysisID
	}

	statement, err := p.getSonarStatement(ceTaskID)
	if err != nil {
		return nil, err
	}
	return statement, nil
}

func (p *Provider) pollTaskUntilSuccess(ceTaskID string, configuredMaxRetries *int, configuredRetryInterval *int) (string, error) {
	if p.client == nil {
		return "", errorutils.CheckErrorf("SonarQube manager is not available")
	}

	maxRetries := defaultMaxRetries
	if configuredMaxRetries != nil {
		maxRetries = *configuredMaxRetries
	}

	retryInterval := defaultRetryInterval
	if configuredRetryInterval != nil {
		retryInterval = *configuredRetryInterval
	}

	pollingInterval := time.Duration(retryInterval) * time.Millisecond
	timeout := time.Duration(maxRetries) * pollingInterval

	pollingExecutor := httputils.PollingExecutor{
		Timeout:         timeout,
		PollingInterval: pollingInterval,
		MsgPrefix:       "Polling SonarQube task",
		PollingAction: func() (shouldStop bool, responseBody []byte, err error) {
			taskDetails, err := p.client.GetTaskDetails(ceTaskID)
			if err != nil {
				return true, nil, err
			}

			switch taskDetails.Task.Status {
			case "SUCCESS":
				log.Info("Task completed successfully with analysis ID:", taskDetails.Task.AnalysisID)
				return true, []byte(taskDetails.Task.AnalysisID), nil
			case "FAILED", "CANCELED":
				return true, nil, errorutils.CheckErrorf("task failed with status: %s", taskDetails.Task.Status)
			}

			log.Debug("Task status:", taskDetails.Task.Status, "continuing to poll...")
			return false, nil, nil
		},
	}

	response, err := pollingExecutor.Execute()
	if err != nil {
		return "", err
	}

	return string(response), nil
}

func (p *Provider) getSonarStatement(ceTaskID string) ([]byte, error) {
	if p.client == nil {
		return nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}

	body, err := p.client.GetSonarIntotoStatement(ceTaskID)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (p *Provider) buildPredicateFromQualityGates() ([]byte, error) {
	if p.client == nil {
		return nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}
	if p.cachedAnalysisId == "" {
		return nil, errorutils.CheckErrorf("analysis id is not available for quality gates")
	}

	qgResponse, err := p.client.GetQualityGateAnalysis(p.cachedAnalysisId)
	if err != nil {
		return nil, err
	}

	return p.mapQualityGatesToPredicate(*qgResponse)
}

// Mapping functions

func (p *Provider) mapQualityGatesToPredicate(qgResponse QualityGatesAnalysis) ([]byte, error) {
	var conditions []struct {
		Status         string `json:"status"`
		MetricKey      string `json:"metricKey"`
		Comparator     string `json:"comparator"`
		ErrorThreshold string `json:"errorThreshold"`
		ActualValue    string `json:"actualValue"`
	}

	for _, condition := range qgResponse.ProjectStatus.Conditions {
		conditions = append(conditions, struct {
			Status         string `json:"status"`
			MetricKey      string `json:"metricKey"`
			Comparator     string `json:"comparator"`
			ErrorThreshold string `json:"errorThreshold"`
			ActualValue    string `json:"actualValue"`
		}{
			Status:         condition.Status,
			MetricKey:      condition.MetricKey,
			Comparator:     condition.Comparator,
			ErrorThreshold: condition.ErrorThreshold,
			ActualValue:    condition.ActualValue,
		})
	}

	predicate := sonarPredicate{
		Gates: []struct {
			Type              string `json:"type"`
			Status            string `json:"status"`
			IgnoredConditions bool   `json:"ignoredConditions"`
			Conditions        []struct {
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
			} `json:"conditions"`
		}{
			{
				Type:              "QUALITY",
				Status:            qgResponse.ProjectStatus.Status,
				IgnoredConditions: qgResponse.ProjectStatus.IgnoredConditions,
				Conditions:        conditions,
			},
		},
	}

	return json.Marshal(predicate)
}
