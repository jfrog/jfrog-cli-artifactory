package sonar

import (
	"os"
	"path/filepath"
	"testing"

	conf "github.com/jfrog/jfrog-cli-artifactory/evidence/config"
	"github.com/stretchr/testify/assert"
)

func withChdir(t *testing.T, dir string) {
	old, _ := os.Getwd()
	assert.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestResolveStatement_NoReportTask(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	resolver := NewStatementResolver()
	_, err := resolver.ResolveStatement()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no report-task.txt file found")
}

// Helpers and factory-based tests

type fakeProvider struct {
	nextResult   []byte
	nextErr      error
	lastCeTaskID string
}

func (f *fakeProvider) GetStatement(ceTaskID string, pollingMaxRetries *int, pollingRetryIntervalMs *int) ([]byte, error) {
	f.lastCeTaskID = ceTaskID
	return f.nextResult, f.nextErr
}

type capturingFactory struct {
	lastURL   string
	lastToken string
	provider  StatementProvider
	retErr    error
}

func (c *capturingFactory) New(sonarURL, token string) (StatementProvider, error) {
	c.lastURL = sonarURL
	c.lastToken = token
	return c.provider, c.retErr
}

func writeReportTask(t *testing.T, dir, serverURL, ceTaskID string) string {
	reportDir := filepath.Join(dir, "target", "sonar")
	assert.NoError(t, os.MkdirAll(reportDir, 0o755))
	path := filepath.Join(reportDir, "report-task.txt")
	content := "serverUrl=" + serverURL + "\nceTaskId=" + ceTaskID + "\n"
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestResolver_ResolveStatementWithConfig_Success(t *testing.T) {
	dir := t.TempDir()
	serverURL := "https://sonar.local"
	ceTaskID := "task-xyz"
	reportPath := writeReportTask(t, dir, serverURL, ceTaskID)

	fprov := &fakeProvider{nextResult: []byte("ok"), nextErr: nil}
	cf := &capturingFactory{provider: fprov}
	res := &statementResolver{providerFactory: cf}

	t.Setenv("SONAR_TOKEN", "token-123")

	cfg := &conf.EvidenceConfig{Sonar: &conf.SonarConfig{ReportTaskFile: reportPath}}
	stmt, err := res.resolveStatementWithConfig(cfg)
	assert.NoError(t, err)
	assert.Equal(t, []byte("ok"), stmt)
	assert.Equal(t, serverURL, cf.lastURL)
	assert.Equal(t, "token-123", cf.lastToken)
	assert.Equal(t, ceTaskID, fprov.lastCeTaskID)
}

func TestResolver_OverridesBaseURLFromConfig(t *testing.T) {
	dir := t.TempDir()
	serverURL := "https://sonar.local"
	overrideURL := "https://override.local"
	ceTaskID := "task-abc"
	reportPath := writeReportTask(t, dir, serverURL, ceTaskID)

	fprov := &fakeProvider{nextResult: []byte("ok2"), nextErr: nil}
	cf := &capturingFactory{provider: fprov}
	res := &statementResolver{providerFactory: cf}

	t.Setenv("SONARQUBE_TOKEN", "fallback-token")
	// Ensure SONAR_TOKEN is empty so fallback is used
	t.Setenv("SONAR_TOKEN", "")

	cfg := &conf.EvidenceConfig{Sonar: &conf.SonarConfig{ReportTaskFile: reportPath, URL: overrideURL}}
	stmt, err := res.resolveStatementWithConfig(cfg)
	assert.NoError(t, err)
	assert.Equal(t, []byte("ok2"), stmt)
	assert.Equal(t, overrideURL, cf.lastURL)
	assert.Equal(t, "fallback-token", cf.lastToken)
	assert.Equal(t, ceTaskID, fprov.lastCeTaskID)
}
