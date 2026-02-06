package commands

import (
	"errors"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
)

func (rbc *ReleaseBundleCreateCommand) createFromArtifacts(lcServicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) (err error) {

	artifactsSource, err := rbc.createArtifactSourceFromSpec()
	if err != nil {
		return err
	}

	return lcServicesManager.CreateReleaseBundleFromArtifactsDraft(rbDetails, queryParams, rbc.signingKeyName, artifactsSource, rbc.draft)
}

func (rbc *ReleaseBundleCreateCommand) createArtifactSourceFromSpec() (services.CreateFromArtifacts, error) {
	var artifactsSource services.CreateFromArtifacts
	rtServicesManager, err := utils.CreateServiceManager(rbc.serverDetails, 3, 0, false)
	if err != nil {
		return artifactsSource, err
	}

	searchResults, callbackFunc, err := utils.SearchFilesBySpecs(rtServicesManager, getArtifactFilesFromSpec(rbc.spec.Files))
	if err != nil {
		return artifactsSource, err
	}

	defer func() {
		if callbackFunc != nil {
			err = errors.Join(err, callbackFunc())
		}
	}()

	artifactsSource, err = aqlResultToArtifactsSource(searchResults)
	if err != nil {
		return artifactsSource, err
	}
	return artifactsSource, nil
}
