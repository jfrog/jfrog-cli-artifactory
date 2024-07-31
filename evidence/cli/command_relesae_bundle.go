package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type evidenceReleaseBundleCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidenceReleaseBundleCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidenceReleaseBundleCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (erc *evidenceReleaseBundleCommand) CreateEvidence(serverDetails *coreConfig.ServerDetails) error {
	createCmd := evidence.NewCreateEvidenceReleaseBundle(
		serverDetails,
		erc.ctx.GetStringFlagValue(predicate),
		erc.ctx.GetStringFlagValue(predicateType),
		erc.ctx.GetStringFlagValue(key),
		erc.ctx.GetStringFlagValue(keyId),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(releaseBundle))
	return erc.execute(createCmd)
}
