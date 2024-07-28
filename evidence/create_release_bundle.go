package evidence

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
)

type CreateEvidenceReleaseBundle struct {
	CreateEvidenceBase
	project       string
	releaseBundle string
}

func NewCreateEvidenceReleaseBundle() *CreateEvidenceReleaseBundle {
	return &CreateEvidenceReleaseBundle{}
}

func (c *CreateEvidenceReleaseBundle) SetServerDetails(serverDetails *config.ServerDetails) *CreateEvidenceReleaseBundle {
	c.serverDetails = serverDetails
	return c
}

func (c *CreateEvidenceReleaseBundle) SetPredicateFilePath(predicateFilePath string) *CreateEvidenceReleaseBundle {
	c.predicateFilePath = predicateFilePath
	return c
}

func (c *CreateEvidenceReleaseBundle) SetPredicateType(predicateType string) *CreateEvidenceReleaseBundle {
	c.predicateType = predicateType
	return c
}

func (c *CreateEvidenceReleaseBundle) SetProject(project string) *CreateEvidenceReleaseBundle {
	c.project = project
	return c
}

func (c *CreateEvidenceReleaseBundle) SetReleaseBundle(releaseBundle string) *CreateEvidenceReleaseBundle {
	c.releaseBundle = releaseBundle
	return c
}

func (c *CreateEvidenceReleaseBundle) SetKey(key string) *CreateEvidenceReleaseBundle {
	c.key = key
	return c
}

func (c *CreateEvidenceReleaseBundle) SetKeyId(keyId string) *CreateEvidenceReleaseBundle {
	c.keyId = keyId
	return c
}

func (c *CreateEvidenceReleaseBundle) CommandName() string {
	return "create-release-bundle-evidence"
}

func (c *CreateEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *CreateEvidenceReleaseBundle) Run() error {
	clientlog.Info("Release Bundle Is Here")
	return nil
}
