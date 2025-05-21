package evidenceproviders

import (
	"errors"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

type EvidenceConfig struct {
	Sonar *SonarConfig `yaml:"sonar,omitempty"`
}

type SonarConfig struct {
	URL            string `yaml:"url"`
	ReportTaskFile string `yaml:"reportTaskFile"`
	MaxRetries     *int   `yaml:"maxRetries"`
	RetryInterval  *int   `yaml:"retryIntervalInSecs"`
	Proxy          string `yaml:"proxy"`
}

func LoadConfig(path string) (map[string]*yaml.Node, error) {
	log.Debug("Loading external provider config", path)
	_, err := fileutils.IsFileExists(path, false)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	evidenceConfig := make(map[string]*yaml.Node)
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = *root.Content[0]
	}
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i].Value
		val := root.Content[i+1]
		evidenceConfig[key] = val
	}
	return evidenceConfig, nil
}

func GetConfig() (map[string]*yaml.Node, error) {
	evidenceDir, err := GetEvidenceDir(false)
	exists, err := fileutils.IsDirExists(evidenceDir, false)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errorutils.CheckError(errors.New("evidence directory does not exist"))
	}
	evidenceConfigFilePath := filepath.Join(evidenceDir, "evidence.yaml")
	fileExists, err := fileutils.IsFileExists(evidenceConfigFilePath, false)
	if err != nil {
		return nil, err
	}
	if !fileExists {
		return nil, errorutils.CheckError(errors.New("evidence.yaml file does not exist"))
	}
	evidenceConfig, err := LoadConfig(evidenceConfigFilePath)
	return evidenceConfig, nil
}

func GetEvidenceDir(global bool) (jfrogDir string, err error) {
	if !global {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		jfrogDir = filepath.Join(wd, ".jfrog")
	} else {
		jfrogDir, err = coreutils.GetJfrogHomeDir()
		if err != nil {
			return "", nil
		}
	}
	return filepath.Join(jfrogDir, "evidence"), nil
}
