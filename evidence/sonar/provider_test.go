package sonar

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

func TestBuildStatement_SuccessDirect(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Return([]byte(`{"ok":true}`), nil)

	stmt, err := provider.GetStatement("task-123", nil, nil)

	assert.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_RetryAfterPolling(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("not ready"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return([]byte(`{"ok":true}`), nil)

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)

	assert.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_PendingThenSuccess(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("not ready"))
	mockManager.On("GetTaskDetails", "task-123").Once().Return(&TaskDetails{Task: struct {
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
	}{Status: "PENDING"}}, nil)
	mockManager.On("GetTaskDetails", "task-123").Once().Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return([]byte(`{"ok":true}`), nil)

	maxRetries := 5
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)

	assert.NoError(t, err)
	assert.Equal(t, []byte(`{"ok":true}`), stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_PollingFailedStatus(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("not ready"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "FAILED"}}, nil)

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)

	assert.Error(t, err)
	assert.Nil(t, stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_PollingSuccessButStatementStillFails(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("not ready"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("still not ready"))

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)

	assert.Error(t, err)
	assert.Nil(t, stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_TaskEndpointError(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("not ready"))
	mockManager.On("GetTaskDetails", "task-123").Return(nil, errors.New("task api error"))

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)

	assert.Error(t, err)
	assert.Nil(t, stmt)
	mockManager.AssertExpectations(t)
}

func TestBuildStatement_EmptyCeTaskID(t *testing.T) {
	provider := &Provider{client: nil}
	stmt, err := provider.GetStatement("", nil, nil)
	assert.Error(t, err)
	assert.Nil(t, stmt)
}

func TestBuildStatement_NilClient(t *testing.T) {
	provider := &Provider{client: nil}
	stmt, err := provider.GetStatement("task-123", nil, nil)
	assert.Error(t, err)
	assert.Nil(t, stmt)
}

func TestBuildStatement_EntitlementMapping_403(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 403: {\"message\":\"JFrog integration is only available on the Enterprise plan\"}"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 403: {\"message\":\"JFrog integration is only available on the Enterprise plan\"}"))

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)
	assert.Error(t, err)
	assert.Nil(t, stmt)
	assert.Contains(t, err.Error(), "Missing entitlement for Sonar evidence creation")
}

func TestBuildStatement_EntitlementMapping_404(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 404: not found"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 404: not found"))

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)
	assert.Error(t, err)
	assert.Nil(t, stmt)
	assert.Contains(t, err.Error(), "Missing entitlement for Sonar evidence creation")
}

func TestBuildStatement_NonEntitlementErrorPassesThrough(t *testing.T) {
	mockManager := &MockSonarManager{}
	provider := &Provider{client: mockManager}

	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 500: boom"))
	mockManager.On("GetTaskDetails", "task-123").Return(&TaskDetails{Task: struct {
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
	}{Status: "SUCCESS"}}, nil)
	mockManager.On("GetSonarIntotoStatement", "task-123").Once().Return(nil, errors.New("enterprise endpoint returned status 500: boom"))

	maxRetries := 1
	retryInterval := 1
	stmt, err := provider.GetStatement("task-123", &maxRetries, &retryInterval)
	assert.Error(t, err)
	assert.Nil(t, stmt)
	assert.Contains(t, err.Error(), "status 500")
}
