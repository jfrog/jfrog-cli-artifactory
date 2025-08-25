package config

import (
	"path/filepath"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/spf13/viper"
)

const (
	jfrogDir         = ".jfrog"
	evidenceDir      = "evidence"
	evidenceFileYml  = "evidence.yml"
	evidenceFileYaml = "evidence.yaml"

	keySonarReportTaskFile    = "sonar.reportTaskFile"
	keySonarURL               = "sonar.url"
	keyPollingMaxRetries      = "sonar.pollingMaxRetries"
	keyPollingRetryIntervalMs = "sonar.pollingRetryIntervalMs"

	envReportTaskFile         = "REPORT_TASK_FILE"
	envSonarURL               = "SONAR_URL"
	envPollingMaxRetries      = "POLLING_MAX_RETRIES"
	envPollingRetryIntervalMs = "POLLING_RETRY_INTERVAL_MS"
)

type SonarConfig struct {
	URL                    string `yaml:"url"`
	ReportTaskFile         string `yaml:"reportTaskFile"`
	PollingMaxRetries      *int   `yaml:"pollingMaxRetries"`
	PollingRetryIntervalMs *int   `yaml:"pollingRetryIntervalMs"`
}

type EvidenceConfig struct {
	Sonar *SonarConfig `yaml:"sonar"`
}

func LoadEvidenceConfig() (*EvidenceConfig, error) {
	// 1) Upstream .jfrog root
	if root, exists, _ := fileutils.FindUpstream(jfrogDir, fileutils.Dir); exists {
		if cfg := readConfigWithEnv(filepath.Join(root, jfrogDir, evidenceDir, evidenceFileYml)); cfg != nil {
			return cfg, nil
		}
		if cfg := readConfigWithEnv(filepath.Join(root, jfrogDir, evidenceDir, evidenceFileYaml)); cfg != nil {
			return cfg, nil
		}
	}

	// 2) Home fallback: ~/.jfrog/evidence/...
	if home, err := coreutils.GetJfrogHomeDir(); err == nil && home != "" {
		if cfg := readConfigWithEnv(filepath.Join(home, evidenceDir, evidenceFileYml)); cfg != nil {
			return cfg, nil
		}
		if cfg := readConfigWithEnv(filepath.Join(home, evidenceDir, evidenceFileYaml)); cfg != nil {
			return cfg, nil
		}
	}

	// 3) Env-only (no file)
	if cfg := readConfigWithEnv(""); cfg != nil {
		return cfg, nil
	}

	return nil, nil
}

func readConfigWithEnv(path string) *EvidenceConfig {
	v := viper.New()

	_ = v.BindEnv(keySonarReportTaskFile, envReportTaskFile)
	_ = v.BindEnv(keySonarURL, envSonarURL)
	_ = v.BindEnv(keyPollingMaxRetries, envPollingMaxRetries)
	_ = v.BindEnv(keyPollingRetryIntervalMs, envPollingRetryIntervalMs)
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		_ = v.ReadInConfig()
	}

	cfg := new(EvidenceConfig)
	if err := v.Unmarshal(&cfg); err != nil {
		_ = errorutils.CheckError(err)
		return nil
	}
	if cfg.Sonar == nil || (*cfg.Sonar == (SonarConfig{})) {
		return nil
	}
	return cfg
}
