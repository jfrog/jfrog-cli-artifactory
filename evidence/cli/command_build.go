package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/create"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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

func (ebc *evidenceBuildCommand) CreateEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	if ebc.ctx.GetStringFlagValue(sigstoreBundle) != "" {
		return errorutils.CheckErrorf("--%s is not supported for build evidence.", sigstoreBundle)
	}

	err := ebc.validateEvidenceBuildContext(ctx)
	if err != nil {
		return err
	}

	createCmd := create.NewCreateEvidenceBuild(
		serverDetails,
		ebc.ctx.GetStringFlagValue(predicate),
		ebc.ctx.GetStringFlagValue(predicateType),
		ebc.ctx.GetStringFlagValue(markdown),
		ebc.ctx.GetStringFlagValue(key),
		ebc.ctx.GetStringFlagValue(keyAlias),
		ebc.ctx.GetStringFlagValue(project),
		ebc.ctx.GetStringFlagValue(buildName),
		ebc.ctx.GetStringFlagValue(buildNumber))
	return ebc.execute(createCmd)
}

func (ebc *evidenceBuildCommand) GetEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	return errorutils.CheckErrorf("Get evidence is not supported with builds")
}

func (ebc *evidenceBuildCommand) VerifyEvidences(ctx *components.Context, serverDetails *config.ServerDetails) error {
	err := ebc.validateEvidenceBuildContext(ctx)
	if err != nil {
		return err
	}

	verifyCmd := verify.NewVerifyEvidencesBuild(
		serverDetails,
		ebc.ctx.GetStringFlagValue(project),
		ebc.ctx.GetStringFlagValue(buildName),
		ebc.ctx.GetStringFlagValue(buildNumber),
		ebc.ctx.GetStringFlagValue(format),
		ebc.ctx.GetStringsArrFlagValue(publicKeys),
		ebc.ctx.GetBoolFlagValue(useArtifactoryKeys),
	)
	return ebc.execute(verifyCmd)
}

func (ebc *evidenceBuildCommand) validateEvidenceBuildContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(buildNumber) || assertValueProvided(ctx, buildNumber) != nil {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a Release Bundle evidence", buildNumber)
	}
	return nil
}
