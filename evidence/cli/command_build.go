package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type evidenceBuildCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidenceBuildCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidenceBuildCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (erc *evidenceBuildCommand) CreateEvidence(serverDetails *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceBuild(
		serverDetails,
		erc.ctx.GetStringFlagValue(predicate),
		erc.ctx.GetStringFlagValue(predicateType),
		erc.ctx.GetStringFlagValue(key),
		erc.ctx.GetStringFlagValue(keyId),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(build))
	return erc.execute(createCmd)
}
