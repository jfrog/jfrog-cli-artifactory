package sonar

import (
	"os"

	conf "github.com/jfrog/jfrog-cli-artifactory/evidence/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type PredicateResolver interface {
	ResolvePredicate() (predicateType string, predicate []byte, markdown []byte, err error)
}

// StatementResolver resolves a full in-toto statement and optional markdown.
type StatementResolver interface {
	ResolveStatement() (statement []byte, err error)
}

type defaultPredicateResolver struct{}

func NewPredicateResolver() PredicateResolver {
	return &defaultPredicateResolver{}
}

func NewStatementResolver() StatementResolver {
	return &defaultPredicateResolver{}
}

func (d *defaultPredicateResolver) ResolvePredicate() (string, []byte, []byte, error) {
	log.Info("Building SonarQube predicate")
	var cfg *conf.EvidenceConfig
	if c, err := conf.LoadEvidenceConfig(); err == nil {
		cfg = c
	}
	return resolvePredicateWithConfig(cfg)
}

func (d *defaultPredicateResolver) ResolveStatement() ([]byte, error) {
	log.Info("Fetching SonarQube in-toto statement")
	var cfg *conf.EvidenceConfig
	if c, err := conf.LoadEvidenceConfig(); err == nil {
		cfg = c
	}
	return resolveStatementWithConfig(cfg)
}

func resolvePredicateWithConfig(cfg *conf.EvidenceConfig) (string, []byte, []byte, error) {
	var reportPath string
	if cfg != nil && cfg.Sonar != nil {
		reportPath = cfg.Sonar.ReportTaskFile
	}
	reportPath = detectReportTaskPath(reportPath)
	if reportPath == "" {
		return "", nil, nil, errorutils.CheckErrorf("no report-task.txt file found and no custom path configured")
	}

	rt, err := parseReportTask(reportPath)
	if err != nil {
		return "", nil, nil, errorutils.CheckErrorf("failed to parse report-task file at %s: %v", reportPath, err)
	}

	log.Info("Parsed report-task file at", reportPath, "with ceTaskID:", rt.CeTaskID, "and projectKey:", rt.ProjectKey)

	sonarBaseURL := resolveSonarBaseURL(rt.CeTaskURL, rt.ServerURL)
	if cfg != nil && cfg.Sonar != nil && cfg.Sonar.URL != "" {
		sonarBaseURL = cfg.Sonar.URL
	}

	token := os.Getenv("SONAR_TOKEN")
	if token == "" {
		token = os.Getenv("SONARQUBE_TOKEN")
	}

	provider, err := NewSonarProviderWithCredentials(sonarBaseURL, token)
	if err != nil {
		return "", nil, nil, err
	}

	// Get polling configuration from config
	var maxRetries, retryInterval *int
	if cfg != nil && cfg.Sonar != nil {
		maxRetries = cfg.Sonar.MaxRetries
		retryInterval = cfg.Sonar.RetryInterval
	}

	predicate, predicateType, markdown, err := provider.BuildPredicate(rt.CeTaskID, rt.AnalysisID, maxRetries, retryInterval)
	if err != nil {
		return "", nil, nil, err
	}

	return predicateType, predicate, markdown, nil
}

func resolveStatementWithConfig(cfg *conf.EvidenceConfig) ([]byte, error) {
	var reportPath string
	if cfg != nil && cfg.Sonar != nil {
		reportPath = cfg.Sonar.ReportTaskFile
	}
	reportPath = detectReportTaskPath(reportPath)
	if reportPath == "" {
		return nil, errorutils.CheckErrorf("no report-task.txt file found and no custom path configured")
	}

	rt, err := parseReportTask(reportPath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to parse report-task file at %s: %v", reportPath, err)
	}

	log.Info("Parsed report-task file at", reportPath, "with ceTaskID:", rt.CeTaskID, "and projectKey:", rt.ProjectKey)

	sonarBaseURL := resolveSonarBaseURL(rt.CeTaskURL, rt.ServerURL)
	if cfg != nil && cfg.Sonar != nil && cfg.Sonar.URL != "" {
		sonarBaseURL = cfg.Sonar.URL
	}

	token := os.Getenv("SONAR_TOKEN")
	if token == "" {
		token = os.Getenv("SONARQUBE_TOKEN")
	}

	provider, err := NewSonarProviderWithCredentials(sonarBaseURL, token)
	if err != nil {
		return nil, err
	}

	var maxRetries, retryInterval *int
	if cfg != nil && cfg.Sonar != nil {
		maxRetries = cfg.Sonar.MaxRetries
		retryInterval = cfg.Sonar.RetryInterval
	}

	statement, err := provider.BuildStatement(rt.CeTaskID, maxRetries, retryInterval)
	if err != nil {
		return nil, err
	}
	return statement, nil
}
