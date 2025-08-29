package sonar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseReportTask_AllKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report-task.txt")
	content := "organization=misha-sonar\nprojectKey=misha-sonar_integration\nserverUrl=https://sonarcloud.io\nserverVersion=8.0.0.66444\ndashboardUrl=https://sonarcloud.io/dashboard?id=misha-sonar_integration\nceTaskId=AZjf8rilT8rlzcpbEtvC\nceTaskUrl=https://sonarcloud.io/api/ce/task?id=AZjf8rilT8rlzcpbEtvC\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rt, err := parseReportTask(p)
	if err != nil {
		t.Fatal(err)
	}
	if rt.CeTaskID != "AZjf8rilT8rlzcpbEtvC" || rt.ProjectKey != "misha-sonar_integration" || rt.ServerURL != "https://sonarcloud.io" {
		t.Fatalf("unexpected parse: %+v", rt)
	}
}

func TestResolveSonarBaseURL(t *testing.T) {
	u := resolveSonarBaseURL("https://sonarcloud.io/api/ce/task?id=abc", "https://sonarcloud.io")
	if u != "https://sonarcloud.io" {
		t.Fatalf("unexpected base url: %s", u)
	}
}
