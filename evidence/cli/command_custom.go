package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type evidenceCustomCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidenceCustomCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidenceCustomCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (ecc *evidenceCustomCommand) CreateEvidence(serverDetails *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceCustom(
		serverDetails,
		ecc.ctx.GetStringFlagValue(EvdPredicate),
		ecc.ctx.GetStringFlagValue(EvdPredicateType),
		ecc.ctx.GetStringFlagValue(EvdKey),
		ecc.ctx.GetStringFlagValue(EvdKeyId),
		ecc.ctx.GetStringFlagValue(EvdRepoPath))
	return ecc.execute(createCmd)
}
