package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type EvidenceReleaseBundleCommand struct {
	c *components.Context
}

func NewEvidenceReleaseBundleCommand(ctx *components.Context) EvidenceCommands {
	return &EvidenceReleaseBundleCommand{
		c: ctx,
	}
}

func (ecc *EvidenceReleaseBundleCommand) CreateEvidence(artifactoryClient *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceReleaseBundle().
		SetServerDetails(artifactoryClient).
		SetPredicateFilePath(ecc.c.GetStringFlagValue(EvdPredicate)).
		SetPredicateType(ecc.c.GetStringFlagValue(EvdPredicateType)).
		SetProject(ecc.c.GetStringFlagValue(project)).
		SetReleaseBundle(ecc.c.GetStringFlagValue(releaseBundle)).
		SetKey(ecc.c.GetStringFlagValue(EvdKey)).
		SetKeyId(ecc.c.GetStringFlagValue(EvdKeyId))
	return commands.Exec(createCmd)
}
