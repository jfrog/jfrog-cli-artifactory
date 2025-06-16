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
func (ecc *evidenceCustomCommand) CreateEvidence(_ *components.Context, serverDetails *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceCustom(
		serverDetails,
		ecc.ctx.GetStringFlagValue(predicate),
		ecc.ctx.GetStringFlagValue(predicateType),
		ecc.ctx.GetStringFlagValue(markdown),
		ecc.ctx.GetStringFlagValue(key),
		ecc.ctx.GetStringFlagValue(keyAlias),
		ecc.ctx.GetStringFlagValue(subjectRepoPath),
		ecc.ctx.GetStringFlagValue(subjectSha256),
		ecc.ctx.GetStringFlagValue(providerId))
	return ecc.execute(createCmd)
}
