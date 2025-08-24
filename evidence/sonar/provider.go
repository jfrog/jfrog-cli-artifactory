package sonar

import (
	"encoding/json"
	"time"

	"github.com/jfrog/jfrog-client-go/config"
	"github.com/jfrog/jfrog-client-go/sonar"
	sonarauth "github.com/jfrog/jfrog-client-go/sonar/auth"
	sonarservices "github.com/jfrog/jfrog-client-go/sonar/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const jfrogPredicateType = "https://jfrog.com/evidence/sonarqube/v1"

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
	manager        sonar.Manager
	cachedAnalysis string
}

// NewSonarProviderWithCredentials creates a new SonarProvider with SonarQube credentials
func NewSonarProviderWithCredentials(sonarURL, token string) (*Provider, error) {
	if sonarURL == "" {
		return nil, errorutils.CheckErrorf("SonarQube URL is required")
	}
	if token == "" {
		return nil, errorutils.CheckErrorf("SonarQube token is required")
	}

	// Create SonarQube auth details
	sonarDetails := sonarauth.NewSonarDetails()
	sonarDetails.SetUrl(sonarURL)
	sonarDetails.SetAccessToken(token)

	// Create config with SonarQube details
	cfg, err := config.NewConfigBuilder().
		SetServiceDetails(sonarDetails).
		Build()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to create SonarQube config: %v", err)
	}

	// Create SonarQube manager
	manager, err := sonar.NewManager(cfg)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to create SonarQube manager: %v", err)
	}

	return &Provider{
		manager: manager,
	}, nil
}

func (p *Provider) BuildPredicate(ceTaskID, analysisID string, maxRetries *int, retryInterval *int) ([]byte, string, []byte, error) {
	// ceTaskID is required for both enterprise endpoint and fallback logic
	if ceTaskID == "" {
		return nil, "", nil, errorutils.CheckErrorf("ceTaskID is required for SonarQube evidence creation")
	}

	// Use cached analysis id if available; otherwise poll once and cache
	if p.cachedAnalysis == "" {
		log.Info("Polling for task completion:", ceTaskID)
		completedAnalysisID, err := p.pollTaskUntilSuccess(ceTaskID, maxRetries, retryInterval)
		if err != nil {
			return nil, "", nil, errorutils.CheckErrorf("failed to poll task completion: %v", err)
		}
		p.cachedAnalysis = completedAnalysisID
	}
	if p.cachedAnalysis != "" {
		analysisID = p.cachedAnalysis
	}

	predicate, predicateType, markdown, err := p.getSonarPredicate(ceTaskID)
	if err != nil {
		log.Debug("Enterprise endpoint failed, falling back to quality gates endpoint:", err.Error())
		predicate, err = p.buildPredicateFromQualityGates(analysisID)
		if err != nil {
			return nil, "", nil, err
		}
		return predicate, jfrogPredicateType, nil, nil
	} else {
		log.Info("Successfully retrieved predicate from enterprise endpoint")
		log.Info("Predicate type:", predicateType)
		if len(markdown) > 0 {
			log.Info("Markdown summary available")
		}
	}
	return predicate, predicateType, markdown, nil
}

// BuildStatement tries to retrieve an in-toto statement from the enterprise endpoint.
// It polls the task until completion. If successful, returns the statement bytes and markdown.
func (p *Provider) BuildStatement(ceTaskID string, maxRetries *int, retryInterval *int) ([]byte, error) {
	if ceTaskID == "" {
		return nil, errorutils.CheckErrorf("ceTaskID is required for SonarQube evidence creation")
	}

	if p.cachedAnalysis == "" {
		log.Info("Polling for task completion:", ceTaskID)
		completedAnalysisID, err := p.pollTaskUntilSuccess(ceTaskID, maxRetries, retryInterval)
		if err != nil {
			return nil, errorutils.CheckErrorf("failed to poll task completion: %v", err)
		}
		p.cachedAnalysis = completedAnalysisID
	}

	statement, err := p.getSonarStatement(ceTaskID)
	if err != nil {
		return nil, err
	}
	return statement, nil
}

func (p *Provider) pollTaskUntilSuccess(ceTaskID string, maxRetries *int, retryInterval *int) (string, error) {
	if p.manager == nil {
		return "", errorutils.CheckErrorf("SonarQube manager is not available")
	}

	// First, check if the task is already completed
	taskDetails, err := p.manager.GetTaskDetails(ceTaskID)
	if err != nil {
		return "", err
	}

	// If task is already completed, return the analysis ID
	switch taskDetails.Task.Status {
	case "SUCCESS":
		log.Info("Task already completed successfully with analysis ID:", taskDetails.Task.AnalysisID)
		return taskDetails.Task.AnalysisID, nil
	case "FAILED", "CANCELED":
		return "", errorutils.CheckErrorf("task failed with status: %s", taskDetails.Task.Status)
	}

	// Task is not completed, start polling
	log.Info("Task not completed, starting polling...")

	// Set default values if not provided
	defaultMaxRetries := 30
	defaultRetryInterval := 5000 // 5 seconds in milliseconds

	if maxRetries != nil {
		defaultMaxRetries = *maxRetries
	}
	if retryInterval != nil {
		defaultRetryInterval = *retryInterval
	}

	// Convert milliseconds to duration
	pollingInterval := time.Duration(defaultRetryInterval) * time.Millisecond
	timeout := time.Duration(defaultMaxRetries) * pollingInterval

	pollingExecutor := httputils.PollingExecutor{
		Timeout:         timeout,
		PollingInterval: pollingInterval,
		MsgPrefix:       "Polling SonarQube task",
		PollingAction: func() (shouldStop bool, responseBody []byte, err error) {
			taskDetails, err := p.manager.GetTaskDetails(ceTaskID)
			if err != nil {
				return true, nil, err
			}

			// Check if task is completed
			switch taskDetails.Task.Status {
			case "SUCCESS":
				log.Info("Task completed successfully with analysis ID:", taskDetails.Task.AnalysisID)
				return true, []byte(taskDetails.Task.AnalysisID), nil
			case "FAILED", "CANCELED":
				return true, nil, errorutils.CheckErrorf("task failed with status: %s", taskDetails.Task.Status)
			}

			// Task is still in progress
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

func (p *Provider) getSonarPredicate(ceTaskID string) ([]byte, string, []byte, error) {
	if p.manager == nil {
		return nil, "", nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}

	// Use raw statement and unmarshal only the needed fields
	body, err := p.manager.GetSonarIntotoStatement(ceTaskID)
	if err != nil {
		return nil, "", nil, err
	}
	var stmt struct {
		Predicate     interface{} `json:"predicate"`
		PredicateType string      `json:"predicateType"`
		Markdown      string      `json:"markdown"`
	}
	if err := json.Unmarshal(body, &stmt); err != nil {
		return nil, "", nil, errorutils.CheckErrorf("failed to parse enterprise statement: %v", err)
	}
	predicateBytes, err := json.Marshal(stmt.Predicate)
	if err != nil {
		return nil, "", nil, errorutils.CheckErrorf("failed to marshal predicate: %v", err)
	}
	return predicateBytes, stmt.PredicateType, []byte(stmt.Markdown), nil
}

func (p *Provider) getSonarStatement(ceTaskID string) ([]byte, error) {
	if p.manager == nil {
		return nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}

	body, err := p.manager.GetSonarIntotoStatement(ceTaskID)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (p *Provider) buildPredicateFromQualityGates(analysisID string) ([]byte, error) {
	if p.manager == nil {
		return nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}

	// Get quality gates response
	qgResponse, err := p.manager.GetQualityGateAnalysis(analysisID)
	if err != nil {
		return nil, err
	}

	return p.mapQualityGatesToPredicate(*qgResponse)
}

// Mapping functions

func (p *Provider) mapQualityGatesToPredicate(qgResponse sonarservices.QualityGatesAnalysis) ([]byte, error) {
	// Map conditions to the new format (removing PeriodIndex as it's not in the new structure)
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
