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

func (erc *EvidenceReleaseBundleCommand) CreateEvidence(artifactoryClient *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceReleaseBundle().
		SetServerDetails(artifactoryClient).
		SetPredicateFilePath(erc.c.GetStringFlagValue(EvdPredicate)).
		SetPredicateType(erc.c.GetStringFlagValue(EvdPredicateType)).
		SetProject(erc.c.GetStringFlagValue(project)).
		SetReleaseBundle(erc.c.GetStringFlagValue(releaseBundle)).
		SetKey(erc.c.GetStringFlagValue(EvdKey)).
		SetKeyId(erc.c.GetStringFlagValue(EvdKeyId))
	return commands.Exec(createCmd)
}
