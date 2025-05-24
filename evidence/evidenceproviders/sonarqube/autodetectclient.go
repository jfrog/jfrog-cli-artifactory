package sonarqube

import (
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

type ReportInfo struct {
	Tool string
	Path string
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DetectBuildToolAndReportFilePath checks for the presence of report-task.txt files in common locations for various build tools.
func DetectBuildToolAndReportFilePath() string {
	candidates := []ReportInfo{
		{"maven", "target/sonar/report-task.txt"},
		{"gradle", "build/sonar/report-task.txt"},
		{"cli", ".scannerwork/report-task.txt"},
		{"msbuild", ".sonarqube/out/.sonar/report-task.txt"},
	}

	for _, c := range candidates {
		if fileExists(c.Path) {
			log.Debug("Found report for", c.Tool, "at", c.Path)
			return c.Path
		}
	}

	log.Debug("No report-task.txt found. Falling back to sonar CLI default.")
	return filepath.Join(".scannerwork/report-task.txt")
}
