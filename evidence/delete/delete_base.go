package delete

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type deleteEvidenceBase struct {
	serverDetails *config.ServerDetails
	evidenceName  string
	pathResolver  SubjectRepoPathResolver
}

func NewDeleteEvidenceBase(serverDetails *config.ServerDetails, evidenceName string, resolver SubjectRepoPathResolver) evidence.Command {
	return &deleteEvidenceBase{
		serverDetails: serverDetails,
		evidenceName:  evidenceName,
		pathResolver:  resolver,
	}
}

type SubjectRepoPathResolver interface {
	ResolveSubjectRepoPath() (string, error)
}

func (d *deleteEvidenceBase) Run() error {
	subjectRepoPath, err := d.pathResolver.ResolveSubjectRepoPath()
	if err != nil {
		return err
	}
	manager, err := utils.CreateEvidenceServiceManager(d.serverDetails, false)
	if err != nil {
		return fmt.Errorf("failed to create evidence service manager: %w", err)
	}
	log.Debug("Deleting evidence for subject:", subjectRepoPath, "and evidence name:", d.evidenceName)
	err = manager.DeleteEvidence(subjectRepoPath, d.evidenceName)
	if err != nil {
		return fmt.Errorf("failed to delete evidence for subject %s and evidence name %s: %w", subjectRepoPath, d.evidenceName, err)
	}
	log.Info("Evidence deleted successfully for subject:", subjectRepoPath, "and evidence name:", d.evidenceName)
	return nil
}

func (v *deleteEvidenceBase) ServerDetails() (*config.ServerDetails, error) {
	return v.serverDetails, nil
}

func (v *deleteEvidenceBase) CommandName() string {
	return "delete-evidence"
}
