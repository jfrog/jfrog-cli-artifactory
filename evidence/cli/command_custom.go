package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type EvidenceCustomCommand struct {
	c *components.Context
}

func NewEvidenceCustomCommand(ctx *components.Context) EvidenceCommands {
	return &EvidenceCustomCommand{
		c: ctx,
	}
}

func (ecc *EvidenceCustomCommand) CreateEvidence(artifactoryClient *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceCustom().
		SetServerDetails(artifactoryClient).
		SetPredicateFilePath(ecc.c.GetStringFlagValue(EvdPredicate)).
		SetPredicateType(ecc.c.GetStringFlagValue(EvdPredicateType)).
		SetRepoPath(ecc.c.GetStringFlagValue(EvdRepoPath)).
		SetKey(ecc.c.GetStringFlagValue(EvdKey)).
		SetKeyId(ecc.c.GetStringFlagValue(EvdKeyId))
	return commands.Exec(createCmd)
}
