package commands

import (
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
)

func (rbc *ReleaseBundleCreateCommand) createFromAql(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) error {
	aqlQuery := rbc.createAqlQueryFromSpec()
	return servicesManager.CreateReleaseBundleFromAqlDraft(rbDetails, queryParams, rbc.signingKeyName, aqlQuery, rbc.draft)
}
