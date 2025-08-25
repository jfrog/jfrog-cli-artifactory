package sonar

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock SonarManager for testing
type MockSonarManager struct {
	mock.Mock
}

func (m *MockSonarManager) GetSonarIntotoStatement(ceTaskID string) ([]byte, error) {
	args := m.Called(ceTaskID)
	if b, ok := args.Get(0).([]byte); ok {
		return b, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockSonarManager) GetQualityGateAnalysis(analysisID string) (*QualityGatesAnalysis, error) {
	args := m.Called(analysisID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if qga, ok := args.Get(0).(*QualityGatesAnalysis); ok {
		return qga, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockSonarManager) GetTaskDetails(ceTaskID string) (*TaskDetails, error) {
	args := m.Called(ceTaskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if td, ok := args.Get(0).(*TaskDetails); ok {
		return td, args.Error(1)
	}
	return nil, args.Error(1)
}

func TestNewSonarProviderWithManager(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}
	assert.NotNil(t, provider)
	assert.IsType(t, &Provider{}, provider)
	assert.Equal(t, mockManager, provider.client)
}

func TestSonarProvider_BuildPredicate_SuccessFromQualityGates(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	// Mock task polling - task is already completed
	taskResp := &TaskDetails{
		Task: struct {
			ID                 string      `json:"id"`
			Type               string      `json:"type"`
			ComponentID        string      `json:"componentId"`
			ComponentKey       string      `json:"componentKey"`
			ComponentName      string      `json:"componentName"`
			ComponentQualifier string      `json:"componentQualifier"`
			AnalysisID         string      `json:"analysisId"`
			Status             string      `json:"status"`
			SubmittedAt        string      `json:"submittedAt"`
			StartedAt          string      `json:"startedAt"`
			ExecutedAt         string      `json:"executedAt"`
			ExecutionTimeMs    int         `json:"executionTimeMs"`
			Logs               interface{} `json:"logs"`
			HasScannerContext  bool        `json:"hasScannerContext"`
			Organization       string      `json:"organization"`
		}{
			ID:         "task-123",
			Status:     "SUCCESS",
			AnalysisID: "analysis-123",
		},
	}

	// Mock quality gates response
	qgResp := &QualityGatesAnalysis{
		ProjectStatus: struct {
			Status     string `json:"status"`
			Conditions []struct {
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				PeriodIndex    int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
			} `json:"conditions"`
			Periods []struct {
				Index int    `json:"index"`
				Mode  string `json:"mode"`
				Date  string `json:"date"`
			} `json:"periods"`
			IgnoredConditions bool `json:"ignoredConditions"`
		}{
			Status: "OK",
			Conditions: []struct {
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				PeriodIndex    int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
			}{
				{
					Status:         "OK",
					MetricKey:      "new_reliability_rating",
					Comparator:     "GT",
					PeriodIndex:    1,
					ErrorThreshold: "1",
					ActualValue:    "1",
				},
			},
			Periods: []struct {
				Index int    `json:"index"`
				Mode  string `json:"mode"`
				Date  string `json:"date"`
			}{
				{
					Index: 1,
					Mode:  "previous_version",
					Date:  "2025-01-15T10:30:00+0000",
				},
			},
			IgnoredConditions: false,
		},
	}

	mockManager.On("GetTaskDetails", "task-123").Return(taskResp, nil)
	mockManager.On("GetQualityGateAnalysis", "analysis-123").Return(qgResp, nil)

	// Use very short polling intervals for testing
	maxRetries := 1
	retryInterval := 1 // 1 millisecond

	predicate, predicateType, err := provider.BuildPredicate(
		"task-123",
		&maxRetries,    // maxRetries
		&retryInterval, // retryInterval
	)

	assert.NoError(t, err)
	assert.NotNil(t, predicate)
	assert.Equal(t, jfrogPredicateType, predicateType)

	// Verify predicate contains expected data
	predicateStr := string(predicate)
	assert.Contains(t, predicateStr, `"type":"QUALITY"`)
	assert.Contains(t, predicateStr, `"status":"OK"`)
	assert.Contains(t, predicateStr, `"ignoredConditions":false`)
	assert.Contains(t, predicateStr, `"metricKey":"new_reliability_rating"`)
	assert.Contains(t, predicateStr, `"comparator":"GT"`)
	assert.Contains(t, predicateStr, `"errorThreshold":"1"`)
	assert.Contains(t, predicateStr, `"actualValue":"1"`)

	mockManager.AssertExpectations(t)
}

func TestSonarProvider_BuildPredicate_BothFail(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	// Mock task polling - task is already completed
	taskResp := &TaskDetails{
		Task: struct {
			ID                 string      `json:"id"`
			Type               string      `json:"type"`
			ComponentID        string      `json:"componentId"`
			ComponentKey       string      `json:"componentKey"`
			ComponentName      string      `json:"componentName"`
			ComponentQualifier string      `json:"componentQualifier"`
			AnalysisID         string      `json:"analysisId"`
			Status             string      `json:"status"`
			SubmittedAt        string      `json:"submittedAt"`
			StartedAt          string      `json:"startedAt"`
			ExecutedAt         string      `json:"executedAt"`
			ExecutionTimeMs    int         `json:"executionTimeMs"`
			Logs               interface{} `json:"logs"`
			HasScannerContext  bool        `json:"hasScannerContext"`
			Organization       string      `json:"organization"`
		}{
			ID:         "task-123",
			Status:     "SUCCESS",
			AnalysisID: "analysis-123",
		},
	}

	// Mock quality gates failure
	mockManager.On("GetTaskDetails", "task-123").Return(taskResp, nil)
	mockManager.On("GetQualityGateAnalysis", "analysis-123").Return(nil, errors.New("quality gates error"))

	// Use very short polling intervals for testing
	maxRetries := 1
	retryInterval := 1 // 1 millisecond

	predicate, predicateType, err := provider.BuildPredicate(
		"task-123",
		&maxRetries,    // maxRetries
		&retryInterval, // retryInterval
	)

	assert.Error(t, err)
	assert.Nil(t, predicate)
	assert.Equal(t, "", predicateType)

	mockManager.AssertExpectations(t)
}

func TestSonarProvider_BuildPredicate_EmptyCeTaskID(t *testing.T) {
	provider := &Provider{client: nil}

	// Test that BuildPredicate returns an error when ceTaskID is empty
	predicate, predicateType, err := provider.BuildPredicate(
		"",  // empty ceTaskID
		nil, // maxRetries
		nil, // retryInterval
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ceTaskID is required")
	assert.Nil(t, predicate)
	assert.Equal(t, "", predicateType)
}

func TestSonarProvider_mapQualityGatesToPredicate(t *testing.T) {
	provider := &Provider{client: nil}

	qgResponse := QualityGatesAnalysis{
		ProjectStatus: struct {
			Status     string `json:"status"`
			Conditions []struct {
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				PeriodIndex    int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
			} `json:"conditions"`
			Periods []struct {
				Index int    `json:"index"`
				Mode  string `json:"mode"`
				Date  string `json:"date"`
			} `json:"periods"`
			IgnoredConditions bool `json:"ignoredConditions"`
		}{
			Status: "OK",
			Conditions: []struct {
				Status         string `json:"status"`
				MetricKey      string `json:"metricKey"`
				Comparator     string `json:"comparator"`
				PeriodIndex    int    `json:"periodIndex"`
				ErrorThreshold string `json:"errorThreshold"`
				ActualValue    string `json:"actualValue"`
			}{
				{
					Status:         "OK",
					MetricKey:      "new_reliability_rating",
					Comparator:     "GT",
					PeriodIndex:    1,
					ErrorThreshold: "1",
					ActualValue:    "1",
				},
			},
			Periods: []struct {
				Index int    `json:"index"`
				Mode  string `json:"mode"`
				Date  string `json:"date"`
			}{
				{
					Index: 1,
					Mode:  "previous_version",
					Date:  "2025-01-15T10:30:00+0000",
				},
			},
			IgnoredConditions: false,
		},
	}

	result, err := provider.mapQualityGatesToPredicate(qgResponse)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify the mapped predicate contains expected data
	resultStr := string(result)
	assert.Contains(t, resultStr, `"type":"QUALITY"`)
	assert.Contains(t, resultStr, `"status":"OK"`)
	assert.Contains(t, resultStr, `"ignoredConditions":false`)
	assert.Contains(t, resultStr, `"metricKey":"new_reliability_rating"`)
	assert.Contains(t, resultStr, `"comparator":"GT"`)
	assert.Contains(t, resultStr, `"errorThreshold":"1"`)
	assert.Contains(t, resultStr, `"actualValue":"1"`)
}
