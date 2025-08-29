package sonar

import (
	"strings"
	"time"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// Set default values if not provided
const defaultMaxRetries = 30
const defaultRetryInterval = 5000 // in milliseconds

type Provider struct {
	client Client
}

type StatementProvider interface {
	GetStatement(ceTaskID string, pollingMaxRetries *int, pollingRetryIntervalMs *int) ([]byte, error)
}

// NewSonarProviderWithCredentials creates a new StatementProvider with SonarQube credentials
func NewSonarProviderWithCredentials(sonarURL, token string) (StatementProvider, error) {
	if sonarURL == "" {
		return nil, errorutils.CheckErrorf("SonarQube URL is required")
	}
	if token == "" {
		return nil, errorutils.CheckErrorf("SonarQube token is required")
	}

	client, err := NewClient(sonarURL, token)
	if err != nil {
		return nil, err
	}
	return &Provider{
		client: client,
	}, nil
}

// GetStatement tries to retrieve an in-toto statement from the integration endpoint.
// If successful, returns the statement bytes.
func (p *Provider) GetStatement(ceTaskID string, pollingMaxRetries *int, pollingRetryIntervalMs *int) ([]byte, error) {
	if ceTaskID == "" {
		return nil, errorutils.CheckErrorf("ceTaskID is required for SonarQube evidence creation")
	}

	if p.client == nil {
		return nil, errorutils.CheckErrorf("SonarQube manager is not available")
	}

	statement, err := p.client.GetSonarIntotoStatement(ceTaskID)
	if err == nil {
		return statement, nil
	}

	err = p.pollTaskUntilSuccess(ceTaskID, pollingMaxRetries, pollingRetryIntervalMs)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to poll task completion: %v", err)
	}

	statement, err = p.client.GetSonarIntotoStatement(ceTaskID)
	if err != nil {
		if isMissingEntitlementError(err) {
			return nil, errorutils.CheckErrorf("Missing entitlement for Sonar evidence creation")
		}
		return nil, err
	}

	return statement, nil
}

func (p *Provider) pollTaskUntilSuccess(ceTaskID string, configuredPollingMaxRetries *int, configuredPollingRetryIntervalMs *int) error {
	if p.client == nil {
		return errorutils.CheckErrorf("SonarQube manager is not available")
	}

	maxRetries := defaultMaxRetries
	if configuredPollingMaxRetries != nil {
		maxRetries = *configuredPollingMaxRetries
	}

	retryIntervalMs := defaultRetryInterval
	if configuredPollingRetryIntervalMs != nil {
		retryIntervalMs = *configuredPollingRetryIntervalMs
	}

	pollingInterval := time.Duration(retryIntervalMs) * time.Millisecond
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
				log.Info("Task completed successfully with taskId:", ceTaskID)
				return true, nil, nil
			case "FAILED", "CANCELED":
				return true, nil, errorutils.CheckErrorf("task failed with status: %s", taskDetails.Task.Status)
			}

			log.Debug("Task status:", taskDetails.Task.Status, "continuing to poll...")
			return false, nil, nil
		},
	}

	_, err := pollingExecutor.Execute()
	if err != nil {
		return err
	}

	return nil
}

func isMissingEntitlementError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "status 404") {
		return true
	}
	if strings.Contains(msg, "status 403") && strings.Contains(msg, "JFrog integration is only available on the Enterprise plan") {
		return true
	}
	return false
}
