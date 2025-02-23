package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type evidenceGitHubCommand struct {
	ctx     *components.Context
	execute execCommandFunc
}

func NewEvidenceGitHubCommand(ctx *components.Context, execute execCommandFunc) EvidenceCommands {
	return &evidenceGitHubCommand{
		ctx:     ctx,
		execute: execute,
	}
}

func (ebc *evidenceGitHubCommand) CreateEvidence(ctx *components.Context, serverDetails *coreConfig.ServerDetails) error {
	err := ebc.validateEvidenceBuildContext(ctx)
	if err != nil {
		return err
	}

	createCmd := evidence.NewCreateGithub(
		serverDetails,
		ebc.ctx.GetStringFlagValue(predicate),
		ebc.ctx.GetStringFlagValue(predicateType),
		ebc.ctx.GetStringFlagValue(markdown),
		ebc.ctx.GetStringFlagValue(key),
		ebc.ctx.GetStringFlagValue(keyAlias),
		ebc.ctx.GetStringFlagValue(project),
		ebc.ctx.GetStringFlagValue(buildName),
		ebc.ctx.GetStringFlagValue(buildNumber),
		ebc.ctx.GetStringFlagValue(typeFlag),
		ebc.ctx.GetStringFlagValue(releaseBundle),
		ebc.ctx.GetStringFlagValue(releaseBundleVersion))
	return ebc.execute(createCmd)
}

func (ebc *evidenceGitHubCommand) validateEvidenceBuildContext(ctx *components.Context) error {
	if !ctx.IsFlagSet(typeFlag) {
		return errorutils.CheckErrorf("--%s is a mandatory field for creating a GitHub evidence", typeFlag)
	}
	if ctx.IsFlagSet(buildName) && assertValueProvided(ctx, buildName) == nil {
		return nil
	}
	if ctx.IsFlagSet(releaseBundle) && assertValueProvided(ctx, releaseBundle) == nil &&
		ctx.IsFlagSet(releaseBundleVersion) && assertValueProvided(ctx, releaseBundleVersion) == nil {
		return nil
	}
	return errorutils.CheckErrorf("Either --build-name or --release-bundle and --release-bundle-version " +
		"are mandatory fields for creating a GitHub evidence")
}
