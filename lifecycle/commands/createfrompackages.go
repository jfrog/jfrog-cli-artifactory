package commands

import (
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

func (rbc *ReleaseBundleCreateCommand) createFromPackages(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) error {

	packageSource := rbc.createPackageSourceFromSpec()

	if len(packageSource.Packages) == 0 {
		return errorutils.CheckErrorf("at least one package is expected in order to create a release bundle from packages")
	}

	return servicesManager.CreateReleaseBundleFromPackages(rbDetails, queryParams, rbc.signingKeyName, packageSource)
}

func (rbc *ReleaseBundleCreateCommand) createPackageSourceFromSpec() services.CreateFromPackagesSource {
	packagesSource := services.CreateFromPackagesSource{}
	for _, file := range rbc.spec.Files {
		if file.Package != "" {
			rbSource := services.PackageSource{
				PackageName:    file.Package,
				PackageVersion: file.Version,
				PackageType:    file.Type,
				RepositoryKey:  file.RepoKey,
			}
			packagesSource.Packages = append(packagesSource.Packages, rbSource)
		}
	}
	return packagesSource
}
