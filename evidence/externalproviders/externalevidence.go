package externalproviders

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/externalproviders/sonarqube"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"os"
	"path/filepath"
)

type EvidenceConfig struct {
	Sonar *sonarqube.SonarConfig `yaml:"sonar,omitempty"`
}

type EvidenceProvider interface {
	// GetEvidence returns the evidence for the given type of external providers evidence
	GetEvidence() ([]byte, error)
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
