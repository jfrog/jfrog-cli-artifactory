package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/create"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
func (ecc *evidenceCustomCommand) CreateEvidence(_ *components.Context, serverDetails *config.ServerDetails) error {
	if ecc.ctx.GetStringFlagValue(sigstoreBundle) != "" && ecc.ctx.GetStringFlagValue(subjectSha256) != "" {
		return errorutils.CheckErrorf("The parameter --%s cannot be used with --%s. The subject hash is extracted from the bundle itself.", subjectSha256, sigstoreBundle)
	}

	// Single command handles both regular evidence creation and sigstore bundles
	createCmd := create.NewCreateEvidenceCustom(
		serverDetails,
		ecc.ctx.GetStringFlagValue(predicate),
		ecc.ctx.GetStringFlagValue(predicateType),
		ecc.ctx.GetStringFlagValue(markdown),
		ecc.ctx.GetStringFlagValue(key),
		ecc.ctx.GetStringFlagValue(keyAlias),
		ecc.ctx.GetStringFlagValue(subjectRepoPath),
		ecc.ctx.GetStringFlagValue(subjectSha256),
		ecc.ctx.GetStringFlagValue(sigstoreBundle),
		ecc.ctx.GetStringFlagValue(providerId))
	return ecc.execute(createCmd)
}

func (ecc *evidenceCustomCommand) VerifyEvidences(_ *components.Context, serverDetails *config.ServerDetails) error {
	verifyCmd := verify.NewVerifyEvidenceCustom(
		serverDetails,
		ecc.ctx.GetStringFlagValue(subjectRepoPath),
		ecc.ctx.GetStringFlagValue(format),
		ecc.ctx.GetStringsArrFlagValue(publicKeys),
		ecc.ctx.GetBoolFlagValue(useArtifactoryKeys),
	)
	return ecc.execute(verifyCmd)
}
