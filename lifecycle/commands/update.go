package commands

import (
	"errors"
	"path"

	coreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	rtServices "github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

type ReleaseBundleUpdateCommand struct {
	releaseBundleCmd
	spec *spec.SpecFiles
	// Multi-builds and multi-bundles sources from command-line flags
	ReleaseBundleSources
}

func NewReleaseBundleUpdateCommand() *ReleaseBundleUpdateCommand {
	return &ReleaseBundleUpdateCommand{}
}

func (rbu *ReleaseBundleUpdateCommand) SetServerDetails(serverDetails *config.ServerDetails) *ReleaseBundleUpdateCommand {
	rbu.serverDetails = serverDetails
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleName(releaseBundleName string) *ReleaseBundleUpdateCommand {
	rbu.releaseBundleName = releaseBundleName
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleVersion(releaseBundleVersion string) *ReleaseBundleUpdateCommand {
	rbu.releaseBundleVersion = releaseBundleVersion
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleProject(rbProjectKey string) *ReleaseBundleUpdateCommand {
	rbu.rbProjectKey = rbProjectKey
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetSpec(spec *spec.SpecFiles) *ReleaseBundleUpdateCommand {
	rbu.spec = spec
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetBuildsSources(sourcesBuilds string) *ReleaseBundleUpdateCommand {
	rbu.sourcesBuilds = sourcesBuilds
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundlesSources(sourcesReleaseBundles string) *ReleaseBundleUpdateCommand {
	rbu.sourcesReleaseBundles = sourcesReleaseBundles
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetSync(sync bool) *ReleaseBundleUpdateCommand {
	rbu.sync = sync
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) CommandName() string {
	return "rb_update"
}

func (rbu *ReleaseBundleUpdateCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbu.serverDetails, nil
}

func (rbu *ReleaseBundleUpdateCommand) Run() error {
	if err := validateArtifactoryVersionSupported(rbu.serverDetails); err != nil {
		return err
	}

	servicesManager, rbDetails, queryParams, err := rbu.getPrerequisites()
	if err != nil {
		return err
	}

	addSources, err := rbu.getAddSources()
	if err != nil {
		return err
	}

	if len(addSources) == 0 {
		return errorutils.CheckErrorf("at least one source must be provided to update a release bundle")
	}

	_, err = servicesManager.UpdateReleaseBundleFromMultipleSources(rbDetails, queryParams, "", addSources)
	return err
}

func (rbu *ReleaseBundleUpdateCommand) getAddSources() ([]services.RbSource, error) {
	// First check if sources are provided via command-line flags
	if rbu.sourcesBuilds != "" || rbu.sourcesReleaseBundles != "" {
		return rbu.buildSourcesFromCommandFlags(), nil
	}

	// Otherwise, parse from spec file
	if rbu.spec == nil || rbu.spec.Files == nil {
		return nil, errorutils.CheckError(errors.New("no spec file or source flags provided"))
	}

	return rbu.buildSourcesFromSpec()
}

func (rbu *ReleaseBundleUpdateCommand) buildSourcesFromCommandFlags() []services.RbSource {
	var sources []services.RbSource

	if rbu.sourcesBuilds != "" {
		sources = buildRbBuildsSources(rbu.sourcesBuilds, rbu.rbProjectKey, sources)
	}

	if rbu.sourcesReleaseBundles != "" {
		sources = buildRbReleaseBundlesSources(rbu.sourcesReleaseBundles, rbu.rbProjectKey, sources)
	}

	return sources
}

func (rbu *ReleaseBundleUpdateCommand) buildSourcesFromSpec() ([]services.RbSource, error) {
	detectedSources, err := detectSourceTypesFromSpec(rbu.spec.Files, true)
	if err != nil {
		return nil, err
	}

	if err = validateCreationSources(detectedSources, true); err != nil {
		return nil, err
	}

	return rbu.convertSpecToSources(detectedSources)
}

func (rbu *ReleaseBundleUpdateCommand) convertSpecToSources(detectedSources []services.SourceType) ([]services.RbSource, error) {
	var sources []services.RbSource
	handledSources := make(SourceTypeSet)

	for _, sourceType := range detectedSources {
		if handledSources[sourceType] {
			continue
		}
		handledSources[sourceType] = true

		switch sourceType {
		case services.ReleaseBundles:
			rbSources, err := rbu.createReleaseBundleSourceFromSpec()
			if err != nil {
				return nil, err
			}
			if len(rbSources.ReleaseBundles) > 0 {
				sources = append(sources, services.RbSource{
					SourceType:     services.ReleaseBundles,
					ReleaseBundles: rbSources.ReleaseBundles,
				})
			}
		case services.Builds:
			buildSources, err := rbu.createBuildSourceFromSpec()
			if err != nil {
				return nil, err
			}
			if len(buildSources.Builds) > 0 {
				sources = append(sources, services.RbSource{
					SourceType: services.Builds,
					Builds:     buildSources.Builds,
				})
			}
		case services.Artifacts:
			artifactSources, err := rbu.createArtifactSourceFromSpec()
			if err != nil {
				return nil, err
			}
			if len(artifactSources.Artifacts) > 0 {
				sources = append(sources, services.RbSource{
					SourceType: services.Artifacts,
					Artifacts:  artifactSources.Artifacts,
				})
			}
		default:
			return nil, errorutils.CheckError(errors.New("unexpected source type: " + string(sourceType)))
		}
	}

	return sources, nil
}

func (rbu *ReleaseBundleUpdateCommand) createReleaseBundleSourceFromSpec() (services.CreateFromReleaseBundlesSource, error) {
	var releaseBundlesSource services.CreateFromReleaseBundlesSource
	for _, file := range rbu.spec.Files {
		if file.Bundle == "" {
			continue
		}
		bundleName, bundleVersion, err := rtServicesUtils.ParseNameAndVersion(file.Bundle, false)
		if err != nil {
			return releaseBundlesSource, err
		}
		if bundleName == "" || bundleVersion == "" {
			return releaseBundlesSource, errorutils.CheckErrorf(
				"bundle name and version are mandatory. Please provide them in the format: 'bundle-name/bundle-version'")
		}
		releaseBundlesSource.ReleaseBundles = append(releaseBundlesSource.ReleaseBundles, services.ReleaseBundleSource{
			ReleaseBundleName:    bundleName,
			ReleaseBundleVersion: bundleVersion,
			ProjectKey:           file.Project,
		})
	}
	return releaseBundlesSource, nil
}

func (rbu *ReleaseBundleUpdateCommand) createBuildSourceFromSpec() (services.CreateFromBuildsSource, error) {
	var buildsSource services.CreateFromBuildsSource
	for _, file := range rbu.spec.Files {
		if file.Build != "" {
			buildName, buildNumber, err := rbu.getBuildDetailsFromIdentifier(file.Build, file.Project)
			if err != nil {
				return services.CreateFromBuildsSource{}, err
			}
			isIncludeDeps, err := file.IsIncludeDeps(false)
			if err != nil {
				return services.CreateFromBuildsSource{}, err
			}
			buildSource := services.BuildSource{
				BuildName:           buildName,
				BuildNumber:         buildNumber,
				BuildRepository:     rtServicesUtils.GetBuildInfoRepositoryByProject(file.Project),
				IncludeDependencies: isIncludeDeps,
			}
			buildsSource.Builds = append(buildsSource.Builds, buildSource)
		}
	}
	return buildsSource, nil
}

func (rbu *ReleaseBundleUpdateCommand) getBuildDetailsFromIdentifier(buildIdentifier, project string) (string, string, error) {
	aqlService, err := rbu.getAqlService()
	if err != nil {
		return "", "", err
	}

	buildName, buildNumber, err := rtServicesUtils.GetBuildNameAndNumberFromBuildIdentifier(buildIdentifier, project, aqlService)
	if err != nil {
		return "", "", err
	}
	if buildName == "" || buildNumber == "" {
		return "", "", errorutils.CheckErrorf("could not identify a build info by the '%s' identifier in artifactory", buildIdentifier)
	}
	return buildName, buildNumber, nil
}

func (rbu *ReleaseBundleUpdateCommand) getAqlService() (*rtServices.AqlService, error) {
	rtServiceManager, err := coreUtils.CreateServiceManager(rbu.serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return rtServices.NewAqlService(rtServiceManager.GetConfig().GetServiceDetails(), rtServiceManager.Client()), nil
}

func (rbu *ReleaseBundleUpdateCommand) createArtifactSourceFromSpec() (services.CreateFromArtifacts, error) {
	var artifactsSource services.CreateFromArtifacts
	rtServicesManager, err := coreUtils.CreateServiceManager(rbu.serverDetails, 3, 0, false)
	if err != nil {
		return artifactsSource, err
	}

	searchResults, callbackFunc, err := coreUtils.SearchFilesBySpecs(rtServicesManager, rbu.getArtifactFilesFromSpec())
	if err != nil {
		return artifactsSource, err
	}

	defer func() {
		if callbackFunc != nil {
			err = errors.Join(err, callbackFunc())
		}
	}()

	artifactsSource, err = rbu.aqlResultToArtifactsSource(searchResults)
	if err != nil {
		return artifactsSource, err
	}
	return artifactsSource, nil
}

func (rbu *ReleaseBundleUpdateCommand) getArtifactFilesFromSpec() []spec.File {
	var artifactFiles []spec.File
	for _, file := range rbu.spec.Files {
		if file.Pattern != "" {
			artifactFiles = append(artifactFiles, file)
		}
	}
	return artifactFiles
}

func (rbu *ReleaseBundleUpdateCommand) aqlResultToArtifactsSource(readers []*content.ContentReader) (artifactsSource services.CreateFromArtifacts, err error) {
	for _, reader := range readers {
		for searchResult := new(rtServicesUtils.ResultItem); reader.NextRecord(searchResult) == nil; searchResult = new(rtServicesUtils.ResultItem) {
			artifactsSource.Artifacts = append(artifactsSource.Artifacts, services.ArtifactSource{
				Path:   path.Join(searchResult.Repo, searchResult.Path, searchResult.Name),
				Sha256: searchResult.Sha256,
			})
		}
		if err = reader.GetError(); err != nil {
			return
		}
		reader.Reset()
	}
	return
}
