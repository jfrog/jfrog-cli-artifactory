package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/commonutils"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/create"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/get"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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

func (erc *evidenceReleaseBundleCommand) CreateEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	if erc.ctx.GetStringFlagValue(sigstoreBundle) != "" {
		return errorutils.CheckErrorf("--%s is not supported for release bundle evidence.", sigstoreBundle)
	}

	err := erc.validateEvidenceReleaseBundleContext(ctx)
	if err != nil {
		return err
	}

	createCmd := create.NewCreateEvidenceReleaseBundle(
		serverDetails,
		erc.ctx.GetStringFlagValue(predicate),
		erc.ctx.GetStringFlagValue(predicateType),
		erc.ctx.GetStringFlagValue(markdown),
		erc.ctx.GetStringFlagValue(key),
		erc.ctx.GetStringFlagValue(keyAlias),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(releaseBundle),
		erc.ctx.GetStringFlagValue(releaseBundleVersion),
		erc.ctx.GetStringFlagValue(providerId),
		erc.ctx.GetStringFlagValue(integration))
	return erc.execute(createCmd)
}

func (erc *evidenceReleaseBundleCommand) GetEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	err := erc.validateEvidenceReleaseBundleContext(ctx)
	if err != nil {
		return err
	}

	getCmd := get.NewGetEvidenceReleaseBundle(
		serverDetails,
		erc.ctx.GetStringFlagValue(releaseBundle),
		erc.ctx.GetStringFlagValue(releaseBundleVersion),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(format),
		erc.ctx.GetStringFlagValue(output),
		erc.ctx.GetStringFlagValue(artifactsLimit),
		erc.ctx.GetBoolFlagValue(includePredicate),
	)
	return erc.execute(getCmd)
}

func (erc *evidenceReleaseBundleCommand) VerifyEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error {
	err := erc.validateEvidenceReleaseBundleContext(ctx)
	if err != nil {
		return err
	}

	verifyCmd := verify.NewVerifyEvidenceReleaseBundle(
		serverDetails,
		erc.ctx.GetStringFlagValue(format),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(releaseBundle),
		erc.ctx.GetStringFlagValue(releaseBundleVersion),
		erc.ctx.GetStringsArrFlagValue(publicKeys),
		erc.ctx.GetBoolFlagValue(useArtifactoryKeys),
	)
	return erc.execute(verifyCmd)
}

func (erc *evidenceReleaseBundleCommand) validateEvidenceReleaseBundleContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(releaseBundleVersion) || assertValueProvided(ctx, releaseBundleVersion) != nil {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a Release Bundle evidence", releaseBundleVersion)
	}
	if ctx.IsFlagSet(artifactsLimit) && !commonutils.IsFlagPositiveNumber(ctx.GetStringFlagValue(artifactsLimit)) {
		return errorutils.CheckErrorf("--%s must be a positive number", artifactsLimit)
	}
	return nil
}
