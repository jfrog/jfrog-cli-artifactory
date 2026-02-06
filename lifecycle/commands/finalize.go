package commands

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type ReleaseBundleFinalizeCommand struct {
	releaseBundleCmd
	signingKeyName string
}

func NewReleaseBundleFinalizeCommand() *ReleaseBundleFinalizeCommand {
	return &ReleaseBundleFinalizeCommand{}
}

func (rbf *ReleaseBundleFinalizeCommand) SetServerDetails(serverDetails *config.ServerDetails) *ReleaseBundleFinalizeCommand {
	rbf.serverDetails = serverDetails
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) SetReleaseBundleName(releaseBundleName string) *ReleaseBundleFinalizeCommand {
	rbf.releaseBundleName = releaseBundleName
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) SetReleaseBundleVersion(releaseBundleVersion string) *ReleaseBundleFinalizeCommand {
	rbf.releaseBundleVersion = releaseBundleVersion
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) SetReleaseBundleProject(rbProjectKey string) *ReleaseBundleFinalizeCommand {
	rbf.rbProjectKey = rbProjectKey
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) SetSigningKeyName(signingKeyName string) *ReleaseBundleFinalizeCommand {
	rbf.signingKeyName = signingKeyName
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) SetSync(sync bool) *ReleaseBundleFinalizeCommand {
	rbf.sync = sync
	return rbf
}

func (rbf *ReleaseBundleFinalizeCommand) CommandName() string {
	return "rb_finalize"
}

func (rbf *ReleaseBundleFinalizeCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbf.serverDetails, nil
}

func (rbf *ReleaseBundleFinalizeCommand) Run() error {
	if err := validateArtifactoryVersionSupported(rbf.serverDetails); err != nil {
		return err
	}

	// Validate Artifactory version supports draft bundle operations (finalize only works on draft bundles)
	if err := ValidateFeatureSupportedVersion(rbf.serverDetails, minArtifactoryVersionForDraftBundleSupport); err != nil {
		return errorutils.CheckErrorf("release bundle finalize requires Artifactory version %s or higher", minArtifactoryVersionForDraftBundleSupport)
	}

	servicesManager, rbDetails, queryParams, err := rbf.getPrerequisites()
	if err != nil {
		return err
	}

	_, err = servicesManager.FinalizeReleaseBundle(rbDetails, queryParams, rbf.signingKeyName)
	return err
}
