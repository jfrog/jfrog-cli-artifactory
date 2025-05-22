package commands

import (
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

func (rbc *ReleaseBundleCreateCommand) createFromPackages(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) error {

	err, packageSource := rbc.createPackageSourceFromSpec()
	if err != nil {
		return err
	}

	if len(packageSource.Packages) == 0 {
		return errorutils.CheckErrorf("at least one package is expected in order to create a release bundle from packages")
	}

	return servicesManager.CreateReleaseBundleFromPackages(rbDetails, queryParams, rbc.signingKeyName, packageSource)
}

func (rbc *ReleaseBundleCreateCommand) createPackageSourceFromSpec() (error, services.CreateFromPackagesSource) {
	var packagesSource services.CreateFromPackagesSource

	packagesSource, err := rbc.convertSpecToPackagesSource(rbc.spec.Files)

	if err != nil {
		return err, packagesSource
	}
	return nil, packagesSource
}

func (rbc *ReleaseBundleCreateCommand) convertSpecToPackagesSource(files []spec.File) (services.CreateFromPackagesSource, error) {
	packagesSource := services.CreateFromPackagesSource{}
	for _, file := range files {
		if file.Package == "" {
			continue
		}

		rbSource := services.PackageSource{
			PackageName:    file.Package,
			PackageVersion: file.Version,
			PackageType:    file.Type,
			RepositoryKey:  file.RepoKey,
		}
		packagesSource.Packages = append(packagesSource.Packages, rbSource)
	}
	return packagesSource, nil
}
