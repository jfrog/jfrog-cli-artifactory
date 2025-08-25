package config

import (
	"path/filepath"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/spf13/viper"
)

type SonarConfig struct {
	URL            string `mapstructure:"url" json:"url" yaml:"url"`
	ReportTaskFile string `mapstructure:"reportTaskFile" json:"reportTaskFile" yaml:"reportTaskFile"`
	MaxRetries     *int   `mapstructure:"maxRetries" json:"maxRetries" yaml:"maxRetries"`
	RetryInterval  *int   `mapstructure:"retryInterval" json:"retryInterval" yaml:"retryInterval"`
}

type EvidenceConfig struct {
	Sonar *SonarConfig `mapstructure:"sonar" json:"sonar" yaml:"sonar"`
}

func LoadEvidenceConfig() (*EvidenceConfig, error) {
	paths := []string{}
	// Project .jfrog path
	paths = append(paths, filepath.Join(".jfrog", "evidence", "evidence.yml"))
	paths = append(paths, filepath.Join(".jfrog", "evidence", "evidence.yaml"))
	// User home .jfrog path
	if home, err := coreutils.GetJfrogHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, "evidence", "evidence.yml"))
		paths = append(paths, filepath.Join(home, "evidence", "evidence.yaml"))
	}

	for _, p := range paths {
		v := viper.New()
		v.SetConfigFile(p)
		if err := v.ReadInConfig(); err != nil {
			continue
		}
		cfg := new(EvidenceConfig)
		if err := v.Unmarshal(&cfg); err != nil {
			return nil, errorutils.CheckError(err)
		}
		return cfg, nil
	}
	return nil, nil
}
