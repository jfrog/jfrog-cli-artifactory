package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/create"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type evidenceApplicationCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidenceApplicationCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidenceApplicationCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (eac *evidenceApplicationCommand) CreateEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	if eac.ctx.GetStringFlagValue(sigstoreBundle) != "" {
		return errorutils.CheckErrorf("--%s is not supported for application evidence.", sigstoreBundle)
	}

	err := eac.validateEvidenceApplicationContext(ctx)
	if err != nil {
		return err
	}

	createCmd := create.NewCreateEvidenceApplication(
		serverDetails,
		eac.ctx.GetStringFlagValue(predicate),
		eac.ctx.GetStringFlagValue(predicateType),
		eac.ctx.GetStringFlagValue(markdown),
		eac.ctx.GetStringFlagValue(key),
		eac.ctx.GetStringFlagValue(keyAlias),
		eac.ctx.GetStringFlagValue(project),
		eac.ctx.GetStringFlagValue(applicationKey),
		eac.ctx.GetStringFlagValue(applicationVersion))
	return eac.execute(createCmd)
}

func (eac *evidenceApplicationCommand) GetEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	return nil
}

func (eac *evidenceApplicationCommand) VerifyEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	return nil
}

func (eac *evidenceApplicationCommand) validateEvidenceApplicationContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(applicationKey) || assertValueProvided(ctx, applicationVersion) != nil {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a Application evidence", releaseBundleVersion)
	}
	return nil
}
