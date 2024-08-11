package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

func (erc *evidenceReleaseBundleCommand) CreateEvidence(ctx *components.Context, serverDetails *coreConfig.ServerDetails) error {
	err := erc.validateEvidenceReleaseBundleContext(ctx)
	if err != nil {
		return err
	}

	createCmd := evidence.NewCreateEvidenceReleaseBundle(
		serverDetails,
		erc.ctx.GetStringFlagValue(predicate),
		erc.ctx.GetStringFlagValue(predicateType),
		erc.ctx.GetStringFlagValue(key),
		erc.ctx.GetStringFlagValue(KeyAlias),
		erc.ctx.GetStringFlagValue(project),
		erc.ctx.GetStringFlagValue(releaseBundle),
		erc.ctx.GetStringFlagValue(releaseBundleVersion))
	return erc.execute(createCmd)
}

func (erc *evidenceReleaseBundleCommand) validateEvidenceReleaseBundleContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(releaseBundleVersion) || assertValueProvided(ctx, releaseBundleVersion) != nil {
		return errorutils.CheckErrorf("'releaseBundleVersion' is a mandatory field for creating a Release Bundle evidence: --%s", releaseBundleVersion)
	}
	return nil
}
