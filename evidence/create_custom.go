package evidence

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type CreateEvidenceCustom struct {
	CreateEvidenceBase
	repoPath string
}

func NewCreateEvidenceCustom() *CreateEvidenceCustom {
	return &CreateEvidenceCustom{}
}

func (c *CreateEvidenceCustom) SetServerDetails(serverDetails *config.ServerDetails) *CreateEvidenceCustom {
	c.serverDetails = serverDetails
	return c
}

func (c *CreateEvidenceCustom) SetPredicateFilePath(predicateFilePath string) *CreateEvidenceCustom {
	c.predicateFilePath = predicateFilePath
	return c
}

func (c *CreateEvidenceCustom) SetPredicateType(predicateType string) *CreateEvidenceCustom {
	c.predicateType = predicateType
	return c
}

func (c *CreateEvidenceCustom) SetRepoPath(repoPath string) *CreateEvidenceCustom {
	c.repoPath = repoPath
	return c
}

func (c *CreateEvidenceCustom) SetKey(key string) *CreateEvidenceCustom {
	c.key = key
	return c
}

func (c *CreateEvidenceCustom) SetKeyId(keyId string) *CreateEvidenceCustom {
	c.keyId = keyId
	return c
}

func (c *CreateEvidenceCustom) CommandName() string {
	return "create-custom-evidence"
}

func (c *CreateEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *CreateEvidenceCustom) Run() error {
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
