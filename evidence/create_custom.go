package evidence

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type createEvidenceCustom struct {
	createEvidenceBase
	subjectRepoPath string
	subjectSha256   string
}

func NewCreateEvidenceCustom(serverDetails *coreConfig.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, subjectRepoPath,
	subjectSha256, providerId string) Command {
	return &createEvidenceCustom{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			providerId:        providerId,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
		},
		subjectRepoPath: subjectRepoPath,
		subjectSha256:   subjectSha256,
	}
}

func (c *createEvidenceCustom) CommandName() string {
	return "create-custom-evidence"
}

func (c *createEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceCustom) Run() error {
	envelope, err := c.createEnvelope(c.subjectRepoPath, c.subjectSha256)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, c.subjectRepoPath)
	if err != nil {
		return err
	}
	return nil
}
