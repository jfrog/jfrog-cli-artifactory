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
	Sonar        *SonarConfig        `yaml:"sonar,omitempty"`
	BuildPublish *BuildPublishConfig `yaml:"buildPublish,omitempty"`
}

type SonarConfig struct {
	URL            string `yaml:"url"`
	ReportTaskFile string `yaml:"reportTaskFile"`
	MaxRetries     *int   `yaml:"maxRetries"`
	RetryInterval  *int   `yaml:"retryIntervalInSecs"`
	Proxy          string `yaml:"proxy"`
}

type BuildPublishConfig struct {
	Enable           bool   `yaml:"enabled" default:"true"`
	EvidenceProvider string `yaml:"evidenceProvider"`
	KeyAlias         string `yaml:"keyAlias"`
	KeyPath          string `yaml:"keyPath"`
}

func (bpc *BuildPublishConfig) IsEnabled() bool {
	if bpc == nil {
		return false
	}
	return bpc.Enable
}

func (bpc *BuildPublishConfig) Validate() error {
	if bpc.EvidenceProvider == "" {
		return errorutils.CheckError(errors.New("evidence provider is not set, evidence provider is the name of the custom evidence provider that will be used to create predicate"))
	}
	if bpc.KeyAlias == "" {
		return errorutils.CheckError(errors.New("key alias is not set, key alias is the name of the key in the keystore that will be used to sign the evidence file"))
	}
	if bpc.KeyPath == "" {
		return errorutils.CheckError(errors.New("key path is not set, key path is the path to the private key file that will be used to sign the evidence file"))
	}
	if exists, err := fileutils.IsFileExists(bpc.KeyPath, false); err != nil || !exists {
		return errorutils.CheckError(errors.New("key path is not a valid file path"))
	}
	return nil
}

func (bpc *BuildPublishConfig) CreateBuildPublishConfig(yamlNode *yaml.Node) (buildPublishConfig *BuildPublishConfig) {
	buildPublishConfig = &BuildPublishConfig{}
	if yamlNode == nil {
		return &BuildPublishConfig{
			Enable:           true,
			EvidenceProvider: "",
			KeyAlias:         "",
			KeyPath:          "",
		}
	}
	if err := yamlNode.Decode(buildPublishConfig); err != nil {
		log.Warn(err)
		return nil
	}
	log.Debug("Creating build publish configuration", buildPublishConfig)
	return buildPublishConfig
}

var ErrEvidenceDirNotExist = errors.New("evidence directory does not exist")

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
		return nil, errorutils.CheckError(ErrEvidenceDirNotExist)
	}
	evidenceConfigFilePath := filepath.Join(evidenceDir, "evidence.yaml")
	fileExists, err := fileutils.IsFileExists(evidenceConfigFilePath, false)
	if err != nil {
		return nil, err
	}
	if !fileExists {
		return nil, err
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
