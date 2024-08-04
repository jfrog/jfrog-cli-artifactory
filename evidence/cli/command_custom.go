package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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
func (ecc *evidenceCustomCommand) CreateEvidence(ctx *components.Context, serverDetails *coreConfig.ServerDetails) error {
	err := ecc.validateEvidenceCustomContext(ctx)
	if err != nil {
		return err
	}
	createCmd := evidence.NewCreateEvidenceCustom(
		serverDetails,
		ecc.ctx.GetStringFlagValue(predicate),
		ecc.ctx.GetStringFlagValue(predicateType),
		ecc.ctx.GetStringFlagValue(key),
		ecc.ctx.GetStringFlagValue(keyId),
		ecc.ctx.GetStringFlagValue(subjectRepoPath),
		ecc.ctx.GetStringFlagValue(subjectSha256))
	return ecc.execute(createCmd)
}

func (ecc *evidenceCustomCommand) validateEvidenceCustomContext(c *components.Context) error {
	if !c.IsFlagSet(subjectSha256) || assertValueProvided(c, subjectSha256) != nil {
		return errorutils.CheckErrorf("'subject-sha256' is a mandatory field for creating a custom evidence: --%s", subjectSha256)
	}
	return nil
}
