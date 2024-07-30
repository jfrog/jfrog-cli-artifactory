package evidence

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type createEvidenceCustom struct {
	createEvidenceBase
	repoPath string
}

func NewCreateEvidenceCustom(serverDetails *coreConfig.ServerDetails, predicateFilePath string, predicateType string, key string, keyId string,
	repoPath string) Command {
	return &createEvidenceCustom{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			key:               key,
			keyId:             keyId,
		},
		repoPath: repoPath,
	}
}

func (c *createEvidenceCustom) CommandName() string {
	return "create-custom-evidence"
}

func (c *createEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceCustom) Run() error {
	envelope, err := c.createEnvelope(c.repoPath)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, c.repoPath)
	if err != nil {
		return err
	}
	return nil
}
