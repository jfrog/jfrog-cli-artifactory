package sonar

import (
	conf "github.com/jfrog/jfrog-cli-artifactory/evidence/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var reportTaskCandidates = []string{
	"target/sonar/report-task.txt",          // maven
	"build/sonar/report-task.txt",           // gradle
	".scannerwork/report-task.txt",          // cli
	".sonarqube/out/.sonar/report-task.txt", // msbuild
}

func GetReportTaskPath() string {
	var cfg *conf.EvidenceConfig
	if c, err := conf.LoadEvidenceConfig(); err == nil {
		cfg = c
	}

	var reportPath string
	if cfg != nil && cfg.Sonar != nil && cfg.Sonar.ReportTaskFile != "" {
		reportPath = cfg.Sonar.ReportTaskFile
	}

	return detectReportTaskPath(reportPath)
}

func detectReportTaskPath(configuredReportPath string) string {
	if configuredReportPath != "" {
		if fileExists(configuredReportPath) {
			log.Debug("Found configured report at", configuredReportPath)
			return configuredReportPath
		}
	}
	for _, path := range reportTaskCandidates {
		if fileExists(path) {
			log.Debug("Found report at", path)
			return path
		}
	}
	log.Debug("No report task file found.")
	return ""
}
