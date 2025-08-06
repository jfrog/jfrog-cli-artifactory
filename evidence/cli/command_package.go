package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/create"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type evidencePackageCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidencePackageCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidencePackageCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (epc *evidencePackageCommand) CreateEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	if epc.ctx.GetStringFlagValue(sigstoreBundle) != "" {
		return errorutils.CheckErrorf("--%s is not supported for package evidence.", sigstoreBundle)
	}

	err := epc.validateEvidencePackageContext(ctx)
	if err != nil {
		return err
	}

	createCmd := create.NewCreateEvidencePackage(
		serverDetails,
		epc.ctx.GetStringFlagValue(predicate),
		epc.ctx.GetStringFlagValue(predicateType),
		epc.ctx.GetStringFlagValue(markdown),
		epc.ctx.GetStringFlagValue(key),
		epc.ctx.GetStringFlagValue(keyAlias),
		epc.ctx.GetStringFlagValue(packageName),
		epc.ctx.GetStringFlagValue(packageVersion),
		epc.ctx.GetStringFlagValue(packageRepoName))
	return epc.execute(createCmd)
}

func (epc *evidencePackageCommand) GetEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	return errorutils.CheckErrorf("Get evidence is not supported with packages")
}

func (epc *evidencePackageCommand) VerifyEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	err := epc.validateEvidencePackageContext(ctx)
	if err != nil {
		return err
	}

	verifyCmd := verify.NewVerifyEvidencePackage(
		serverDetails,
		epc.ctx.GetStringFlagValue(format),
		epc.ctx.GetStringFlagValue(packageName),
		epc.ctx.GetStringFlagValue(packageVersion),
		epc.ctx.GetStringFlagValue(packageRepoName),
		epc.ctx.GetStringsArrFlagValue(publicKeys),
		epc.ctx.GetBoolFlagValue(useArtifactoryKeys),
	)
	return epc.execute(verifyCmd)
}

func (epc *evidencePackageCommand) validateEvidencePackageContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(packageVersion) || assertValueProvided(ctx, packageVersion) != nil {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a Package evidence", packageVersion)
	}
	if !ctx.IsFlagSet(packageRepoName) || assertValueProvided(ctx, packageRepoName) != nil {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a Package evidence", packageRepoName)
	}
	return nil
}
