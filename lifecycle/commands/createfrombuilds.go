package commands

import (
	"encoding/json"
	"sync"

	rtServices "github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
)

func (rbc *ReleaseBundleCreateCommand) createFromBuilds(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) error {

	buildsSource, err := rbc.createBuildSourceFromSpec()
	if err != nil {
		return err
	}

	if len(buildsSource.Builds) == 0 {
		return errorutils.CheckErrorf("at least one build is expected in order to create a release bundle from builds")
	}

	return servicesManager.CreateReleaseBundleFromBuildsDraft(rbDetails, queryParams, rbc.signingKeyName, buildsSource, rbc.draft)
}

func (rbc *ReleaseBundleCreateCommand) createBuildSourceFromSpec() (buildsSource services.CreateFromBuildsSource, err error) {
	if rbc.buildsSpecPath != "" {
		buildsSource, err = rbc.getBuildSourceFromBuildsSpec()
	} else {
		buildsSource, err = convertSpecToBuildsSource(rbc.serverDetails, rbc.spec.Files)
	}
	return buildsSource, err
}

func (rbc *ReleaseBundleCreateCommand) getBuildSourceFromBuildsSpec() (buildsSource services.CreateFromBuildsSource, err error) {
	builds := CreateFromBuildsSpec{}
	content, err := fileutils.ReadFile(rbc.buildsSpecPath)
	if err != nil {
		return
	}
	if err = json.Unmarshal(content, &builds); errorutils.CheckError(err) != nil {
		return
	}

	return rbc.convertBuildsSpecToBuildsSource(builds)
}

func (rbc *ReleaseBundleCreateCommand) convertBuildsSpecToBuildsSource(builds CreateFromBuildsSpec) (services.CreateFromBuildsSource, error) {
	// Create the AQL service once and reuse across all lookups.
	aqlService, err := getAqlService(rbc.serverDetails)
	if err != nil {
		return services.CreateFromBuildsSource{}, err
	}

	// Fan out build-number lookups in parallel; preserve order via indexed slice.
	buildSources := make([]services.BuildSource, len(builds.Builds))
	errs := make([]error, len(builds.Builds))
	var wg sync.WaitGroup
	for i, build := range builds.Builds {
		wg.Add(1)
		go func(idx int, b SourceBuildSpec) {
			defer wg.Done()
			buildNumber, lookupErr := rbc.getLatestBuildNumberIfEmpty(b.Name, b.Number, b.Project, aqlService)
			if lookupErr != nil {
				errs[idx] = lookupErr
				return
			}
			buildSources[idx] = services.BuildSource{
				BuildName:           b.Name,
				BuildNumber:         buildNumber,
				BuildRepository:     utils.GetBuildInfoRepositoryByProject(b.Project),
				IncludeDependencies: b.IncludeDependencies,
			}
		}(i, build)
	}
	wg.Wait()

	for _, e := range errs {
		if e != nil {
			return services.CreateFromBuildsSource{}, e
		}
	}
	return services.CreateFromBuildsSource{Builds: buildSources}, nil
}

func (rbc *ReleaseBundleCreateCommand) getLatestBuildNumberIfEmpty(buildName, buildNumber, project string, aqlService *rtServices.AqlService) (string, error) {
	if buildNumber != "" {
		return buildNumber, nil
	}

	buildNumber, err := utils.GetLatestBuildNumberFromArtifactory(buildName, project, aqlService)
	if err != nil {
		return "", err
	}
	if buildNumber == "" {
		return "", errorutils.CheckErrorf("could not find a build info with name '%s' in artifactory", buildName)
	}
	return buildNumber, nil
}

type CreateFromBuildsSpec struct {
	Builds []SourceBuildSpec `json:"builds,omitempty"`
}

type SourceBuildSpec struct {
	Name                string `json:"name,omitempty"`
	Number              string `json:"number,omitempty"`
	Project             string `json:"project,omitempty"`
	IncludeDependencies bool   `json:"includeDependencies,omitempty"`
}
