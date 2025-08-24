package sonar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseReportTask_AllKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report-task.txt")
	content := "ceTaskUrl=https://sonarcloud.io/api/ce/task?id=abc123\nceTaskId=abc123\nanalysisId=ana456\nprojectKey=my:proj\nserverUrl=https://sonarcloud.io\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rt, err := parseReportTask(p)
	if err != nil {
		t.Fatal(err)
	}
	if rt.CeTaskID != "abc123" || rt.AnalysisID != "ana456" || rt.ProjectKey != "my:proj" || rt.ServerURL != "https://sonarcloud.io" {
		t.Fatalf("unexpected parse: %+v", rt)
	}
}

func TestResolveSonarBaseURL(t *testing.T) {
	u := resolveSonarBaseURL("https://sonarcloud.io/api/ce/task?id=abc", "https://sonarcloud.io")
	if u != "https://sonarcloud.io" {
		t.Fatalf("unexpected base url: %s", u)
	}
}
