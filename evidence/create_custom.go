package evidence

import (
	"encoding/json"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	evidenceService "github.com/jfrog/jfrog-client-go/evidence/services"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
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
	evidenceManager, err := utils.CreateEvidenceServiceManager(c.serverDetails, false)
	if err != nil {
		return err
	}

	evidenceDetails := evidenceService.EvidenceDetails{
		SubjectUri:  strings.Split(c.repoPath, "@")[0],
		DSSEFileRaw: envelope,
	}
	body, err := evidenceManager.UploadEvidence(evidenceDetails)
	if err != nil {
		return err
	}

	createResponse := &model.CreateResponse{}
	err = json.Unmarshal(body, createResponse)
	if err != nil {
		return err
	}
	if createResponse.Verified {
		clientlog.Info("Evidence successfully created and verified")
		return nil
	}
	clientlog.Info("Evidence successfully created but not verified due to missing/invalid public key")
	return nil
}
